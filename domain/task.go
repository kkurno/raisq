package domain

import "fmt"

type State string

const (
	Queued  State = "queued"
	Running State = "running"
	Ready   State = "ready"
	Parked  State = "parked"
	Done    State = "done"
)

type ParkedReason string

const (
	OutOfAttempts  ParkedReason = "out of attempts"
	Question       ParkedReason = "question"
	NeedsSomething ParkedReason = "needs something"
	Stuck          ParkedReason = "stuck"
	Blocked        ParkedReason = "blocked"
)

// TaskState is what state.yaml holds.
type TaskState struct {
	State  State        `yaml:"state" json:"state"`
	Reason ParkedReason `yaml:"reason,omitempty" json:"reason,omitempty"`
	Note   string       `yaml:"note,omitempty" json:"note,omitempty"`
	Added  string       `yaml:"added" json:"added"`
	// Base is the commit the task's branch started from: "before the task".
	Base string `yaml:"base,omitempty" json:"base,omitempty"`
	// Branch defaults to raisq/<task>. The parts of a split task share one.
	Branch string `yaml:"branch,omitempty" json:"branch,omitempty"`
	// PartOf names the whole task this one is a part of. A part that passes is
	// reviewed with the whole, not on its own.
	PartOf string `yaml:"part_of,omitempty" json:"part_of,omitempty"`
	// After lists tasks that must be done before this one may start.
	After []string `yaml:"after,omitempty" json:"after,omitempty"`
	// CheckOnly runs no builder: the whole of a split task is checked as it
	// stands on the branch its parts built.
	CheckOnly bool `yaml:"check_only,omitempty" json:"check_only,omitempty"`
	// LastRun is the run folder name of the latest run.
	LastRun string `yaml:"last_run,omitempty" json:"last_run,omitempty"`
}

var transitions = map[State][]State{
	Queued:  {Running},
	Running: {Ready, Parked, Queued},
	Ready:   {Done, Queued},
	Parked:  {Queued},
	Done:    {},
}

// Move enforces the task lifecycle. A Running task goes back to Queued only
// when a fact it relied on changed.
func (s TaskState) Move(to State) (TaskState, error) {
	for _, ok := range transitions[s.State] {
		if ok == to {
			s.State = to
			if to != Parked {
				s.Reason, s.Note = "", ""
			}
			return s, nil
		}
	}
	return s, fmt.Errorf("a %s task can't become %s", s.State, to)
}

// ResultStatus is what the builder writes in result.json.
type ResultStatus string

const (
	ResultDone           ResultStatus = "done"
	ResultQuestion       ResultStatus = "question"
	ResultNeedsSomething ResultStatus = "needs-something"
	ResultFailed         ResultStatus = "failed"
)

type Result struct {
	Status   ResultStatus `json:"status"`
	Question string       `json:"question,omitempty"`
	Needs    string       `json:"needs,omitempty"`
	Summary  string       `json:"summary,omitempty"`
}

func (r Result) Validate() error {
	switch r.Status {
	case ResultDone, ResultFailed:
	case ResultQuestion:
		if r.Question == "" {
			return fmt.Errorf("result: question without the question")
		}
	case ResultNeedsSomething:
		if r.Needs == "" {
			return fmt.Errorf("result: needs-something without saying what")
		}
	default:
		return fmt.Errorf("result: unknown status %q", r.Status)
	}
	return nil
}

// Choice is one decision the spec didn't cover.
type Choice struct {
	What string `json:"what"`
	Why  string `json:"why,omitempty"`
}

// Failure is one thing wrong with an attempt, as the handoff carries it.
type Failure struct {
	Source string `json:"source"` // a check name, "critic", or "audit"
	What   string `json:"what"`
	Lines  string `json:"lines,omitempty"`
}

// Handoff is what the next attempt is told.
type Handoff struct {
	Attempt  int       `json:"attempt"`
	Failures []Failure `json:"failures"`
	Changed  []string  `json:"changed"`
	Note     string    `json:"note,omitempty"`
}

// SameAs reports whether two handoffs fail the same way, for the stuck rule.
func (h Handoff) SameAs(o Handoff) bool {
	if len(h.Failures) != len(o.Failures) || len(h.Failures) == 0 {
		return false
	}
	seen := map[string]int{}
	for _, f := range h.Failures {
		seen[f.Source+"\x00"+f.What]++
	}
	for _, f := range o.Failures {
		k := f.Source + "\x00" + f.What
		if seen[k] == 0 {
			return false
		}
		seen[k]--
	}
	return true
}

// Lines keeps the last n lines of a check's output for the handoff; the full
// log stays in the run.
func TailLines(s string, n int) string {
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' && i != len(s)-1 {
			count++
			if count == n {
				return s[i+1:]
			}
		}
	}
	return s
}
