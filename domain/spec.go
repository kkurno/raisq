// Package domain holds RaisQ's rules. It imports nothing from RaisQ and
// nothing that touches files, processes or the network.
package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Tag string

const (
	Confirmed Tag = "confirmed"
	Inferred  Tag = "inferred"
	Gap       Tag = "gap"
)

// KindRule marks a constraint rather than a world fact. Rules never expire.
const KindRule = "rule"

// Fact is one item in facts/.
type Fact struct {
	ID      string        `yaml:"id" json:"id"`
	Text    string        `yaml:"text,omitempty" json:"text,omitempty"`
	Kind    string        `yaml:"kind,omitempty" json:"kind,omitempty"`
	Tag     Tag           `yaml:"tag" json:"tag"`
	SaidBy  string        `yaml:"said_by,omitempty" json:"said_by,omitempty"`
	SaidOn  string        `yaml:"said_on,omitempty" json:"said_on,omitempty"`
	Needs   string        `yaml:"needs,omitempty" json:"needs,omitempty"`
	Recheck time.Duration `yaml:"-" json:"-"`
	// RecheckText is the per-item override as written, such as "90d".
	RecheckText string `yaml:"recheck,omitempty" json:"recheck,omitempty"`
	See         string `yaml:"see,omitempty" json:"see,omitempty"`
	Topic       string `yaml:"-" json:"topic,omitempty"`
}

func (f Fact) Validate() error {
	if f.ID == "" {
		return fmt.Errorf("fact without an id")
	}
	switch f.Tag {
	case Confirmed, Inferred:
		if f.Text == "" && f.See == "" {
			return fmt.Errorf("fact %s: %s needs text", f.ID, f.Tag)
		}
	case Gap:
	default:
		return fmt.Errorf("fact %s: unknown tag %q", f.ID, f.Tag)
	}
	if f.Tag == Confirmed && f.Needs == "stakeholder" && !strings.Contains(strings.ToLower(f.SaidBy), "stakeholder") {
		return fmt.Errorf("fact %s: needs the stakeholder, but was confirmed by %q", f.ID, f.SaidBy)
	}
	return nil
}

// EffectiveTag is the tag a verdict sees: a confirmed fact past its recheck
// date counts as inferred until someone reconfirms it.
func (f Fact) EffectiveTag(now time.Time, periods map[string]time.Duration) Tag {
	if f.Tag != Confirmed || f.Kind == KindRule {
		return f.Tag
	}
	period := f.Recheck
	if period == 0 {
		period = periods[f.Kind]
	}
	if period == 0 || f.SaidOn == "" {
		return f.Tag
	}
	said, err := time.Parse("2006-01-02", f.SaidOn)
	if err != nil {
		return f.Tag
	}
	if now.After(said.Add(period)) {
		return Inferred
	}
	return f.Tag
}

// Facts is one revision of facts/.
type Facts struct {
	Revision string
	Items    []Fact
}

func (fs Facts) Get(id string) (Fact, bool) {
	for _, f := range fs.Items {
		if f.ID == id {
			if f.See != "" && f.Text == "" {
				if target, ok := fs.Get(f.See); ok {
					target.ID = f.ID
					return target, true
				}
			}
			return f, true
		}
	}
	return Fact{}, false
}

func (fs Facts) Validate() error {
	seen := map[string]bool{}
	for _, f := range fs.Items {
		if err := f.Validate(); err != nil {
			return err
		}
		if seen[f.ID] {
			return fmt.Errorf("fact %s appears twice", f.ID)
		}
		seen[f.ID] = true
	}
	for _, f := range fs.Items {
		if f.See != "" && !seen[f.See] {
			return fmt.Errorf("fact %s: see %s, which doesn't exist", f.ID, f.See)
		}
	}
	return nil
}

// Index is one line per fact, grouped by topic: what the builder, the critic
// and the spec writer read instead of every fact.
func (fs Facts) Index() string {
	byTopic := map[string][]Fact{}
	var topics []string
	for _, f := range fs.Items {
		if _, ok := byTopic[f.Topic]; !ok {
			topics = append(topics, f.Topic)
		}
		byTopic[f.Topic] = append(byTopic[f.Topic], f)
	}
	sort.Strings(topics)
	var b strings.Builder
	for _, t := range topics {
		name := t
		if name == "" {
			name = "(no topic)"
		}
		fmt.Fprintf(&b, "## %s\n", name)
		for _, f := range byTopic[t] {
			line := f.Text
			switch {
			case f.Tag == Gap:
				line = "(unknown: leave it out)"
			case f.See != "" && f.Text == "":
				line = "same as " + f.See
			}
			fmt.Fprintf(&b, "- %s [%s]: %s\n", f.ID, f.Tag, firstLine(line))
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Criterion is one acceptance criterion in a task spec.
type Criterion struct {
	ID   string `yaml:"id" json:"id"`
	Text string `yaml:"text" json:"text"`
}

// TaskSpec is one task's acceptance criteria.
type TaskSpec struct {
	Name     string      `yaml:"name" json:"name"`
	Title    string      `yaml:"title" json:"title"`
	Criteria []Criterion `yaml:"criteria" json:"criteria"`
	// RestsOn is the facts the spec writer said the draft depends on.
	RestsOn []string `yaml:"rests_on,omitempty" json:"rests_on,omitempty"`
	Source  string   `yaml:"source,omitempty" json:"source,omitempty"`
	// Outside marks text that came from a tracker or a spec tool.
	Outside bool `yaml:"outside,omitempty" json:"outside,omitempty"`
	// ChangesAPI says the task may break a library's public API. Without it,
	// a removed or changed export fails the API check.
	ChangesAPI bool `yaml:"changes_api,omitempty" json:"changes_api,omitempty"`
}

func (s TaskSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("task spec without a name")
	}
	if len(s.Criteria) == 0 {
		return fmt.Errorf("task %s has no acceptance criteria", s.Name)
	}
	seen := map[string]bool{}
	for _, c := range s.Criteria {
		if c.ID == "" || c.Text == "" {
			return fmt.Errorf("task %s: a criterion needs an id and text", s.Name)
		}
		if seen[c.ID] {
			return fmt.Errorf("task %s: criterion %s appears twice", s.Name, c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}
