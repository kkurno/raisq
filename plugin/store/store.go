// Package store keeps .raisq/ as plain files.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/domain"
)

type Files struct {
	root string // the .raisq directory
	// periods come from the preset; RaisQ ships the defaults.
	periods map[string]time.Duration
}

var DefaultPeriods = map[string]time.Duration{
	"contact": 180 * day,
	"hours":   90 * day,
	"price":   30 * day,
	"terms":   90 * day,
}

const day = 24 * time.Hour

func New(root string) *Files {
	return &Files{root: root, periods: DefaultPeriods}
}

func (s *Files) Root() string                      { return s.root }
func (s *Files) Periods() map[string]time.Duration { return s.periods }
func (s *Files) path(p ...string) string           { return filepath.Join(append([]string{s.root}, p...)...) }

// --- facts ---

func (s *Files) LoadFacts() (domain.Facts, error) {
	return s.loadFactsFrom(s.path("facts"))
}

func (s *Files) loadFactsFrom(dir string) (domain.Facts, error) {
	var out domain.Facts
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var items []domain.Fact
		if err := yaml.Unmarshal(data, &items); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		rel, _ := filepath.Rel(dir, p)
		topic := strings.TrimSuffix(filepath.ToSlash(rel), ".yaml")
		for _, f := range items {
			f.Topic = topic
			if f.RecheckText != "" {
				d, err := ParseDays(f.RecheckText)
				if err != nil {
					return fmt.Errorf("%s: fact %s: %w", p, f.ID, err)
				}
				f.Recheck = d
			}
			out.Items = append(out.Items, f)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, out.Validate()
}

// ParseDays reads "90d" or "12w".
func ParseDays(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("bad recheck %q", s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, fmt.Errorf("bad recheck %q", s)
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * day, nil
	case 'w':
		return time.Duration(n) * 7 * day, nil
	}
	return 0, fmt.Errorf("bad recheck %q: use d or w", s)
}

// SaveFact adds or replaces a fact in its topic file. The old version is kept
// in history by the next pin.
func (s *Files) SaveFact(f domain.Fact, topic string) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if topic == "" {
		topic = strings.SplitN(f.ID, "/", 2)[0]
	}
	p := s.path("facts", topic+".yaml")
	var items []domain.Fact
	if data, err := os.ReadFile(p); err == nil {
		if err := yaml.Unmarshal(data, &items); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	replaced := false
	for i := range items {
		if items[i].ID == f.ID {
			items[i], replaced = f, true
		}
	}
	if !replaced {
		items = append(items, f)
	}
	return writeYAML(p, items)
}

// PinFacts records the current facts as a revision. A revision is the hash
// of the files' content, so an unchanged folder pins to the same revision and
// a hand edit gets a new one whether or not it was committed.
func (s *Files) PinFacts() (domain.Facts, error) {
	facts, err := s.LoadFacts()
	if err != nil {
		return facts, err
	}
	files, err := s.snapshot(s.path("facts"))
	if err != nil {
		return facts, err
	}
	rev := hashFiles(files)
	dir := s.path("history", "facts", rev)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		for name, data := range files {
			if err := writeFile(filepath.Join(dir, name), data); err != nil {
				return facts, err
			}
		}
		if len(files) == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return facts, err
			}
		}
	}
	facts.Revision = rev
	return facts, nil
}

func (s *Files) FactsAt(rev string) (domain.Facts, error) {
	dir := s.path("history", "facts", rev)
	if _, err := os.Stat(dir); err != nil {
		return domain.Facts{}, fmt.Errorf("facts revision %s not found", rev)
	}
	f, err := s.loadFactsFrom(dir)
	f.Revision = rev
	return f, err
}

func (s *Files) snapshot(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

func hashFiles(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(files[n]))
		h.Write(files[n])
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// --- rubrics ---

func (s *Files) LoadRubrics() ([]domain.Rubric, error) {
	var out []domain.Rubric
	err := filepath.WalkDir(s.path("rubrics"), func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var rs []domain.Rubric
		if err := yaml.Unmarshal(data, &rs); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, rs...)
		return nil
	})
	return out, err
}

func (s *Files) SaveRubric(r domain.Rubric, file string) error {
	p := s.path("rubrics", file+".yaml")
	var rs []domain.Rubric
	if data, err := os.ReadFile(p); err == nil {
		if err := yaml.Unmarshal(data, &rs); err != nil {
			return err
		}
	}
	replaced := false
	for i := range rs {
		if rs[i].ID == r.ID {
			rs[i], replaced = r, true
		}
	}
	if !replaced {
		rs = append(rs, r)
	}
	return writeYAML(p, rs)
}

// --- tasks ---

func (s *Files) Tasks() ([]string, error) {
	entries, err := os.ReadDir(s.path("tasks"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, err
}

func (s *Files) LoadSpec(task string) (domain.TaskSpec, error) {
	var spec domain.TaskSpec
	if err := readYAML(s.path("tasks", task, "spec.yaml"), &spec); err != nil {
		return spec, err
	}
	return spec, spec.Validate()
}

func (s *Files) SaveSpec(spec domain.TaskSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	return writeYAML(s.path("tasks", spec.Name, "spec.yaml"), spec)
}

func (s *Files) PinSpec(task string) (string, error) {
	data, err := os.ReadFile(s.path("tasks", task, "spec.yaml"))
	if err != nil {
		return "", err
	}
	rev := hashFiles(map[string][]byte{"spec.yaml": data})
	p := s.path("tasks", task, "history", rev+".yaml")
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		if err := writeFile(p, data); err != nil {
			return "", err
		}
	}
	return rev, nil
}

func (s *Files) SpecAt(task, rev string) (domain.TaskSpec, error) {
	var spec domain.TaskSpec
	err := readYAML(s.path("tasks", task, "history", rev+".yaml"), &spec)
	return spec, err
}

func (s *Files) LoadState(task string) (domain.TaskState, error) {
	var st domain.TaskState
	err := readYAML(s.path("tasks", task, "state.yaml"), &st)
	return st, err
}

func (s *Files) SaveState(task string, st domain.TaskState) error {
	return writeYAML(s.path("tasks", task, "state.yaml"), st)
}

// --- runs ---

func (s *Files) NewRun(task string) (string, error) {
	base := time.Now().UTC().Format("2006-01-02T15-04-05")
	name := base
	for i := 2; ; i++ {
		if _, err := os.Stat(s.RunDir(task, name)); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name, os.MkdirAll(s.RunDir(task, name), 0o755)
}

func (s *Files) RunDir(task, run string) string { return s.path("tasks", task, "runs", run) }

// SaveRunFile writes JSON for .json names, and raw text for anything else.
func (s *Files) SaveRunFile(task, run, name string, data any) error {
	p := filepath.Join(s.RunDir(task, run), name)
	if str, ok := data.(string); ok && !strings.HasSuffix(name, ".json") {
		return writeFile(p, []byte(str))
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(p, append(b, '\n'))
}

func (s *Files) LoadRunFile(task, run, name string, into any) error {
	data, err := os.ReadFile(filepath.Join(s.RunDir(task, run), name))
	if err != nil {
		return err
	}
	if str, ok := into.(*string); ok {
		*str = string(data)
		return nil
	}
	return json.Unmarshal(data, into)
}

// --- baselines ---

func (s *Files) Baseline(commit string) (domain.Baseline, bool, error) {
	var b domain.Baseline
	data, err := os.ReadFile(s.path("baselines", commit+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return b, false, nil
	}
	if err != nil {
		return b, false, err
	}
	return b, true, json.Unmarshal(data, &b)
}

func (s *Files) SaveBaseline(b domain.Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(s.path("baselines", b.Commit+".json"), data)
}

func (s *Files) Snapshot(commit, extractor string) (map[string]string, bool, error) {
	var m map[string]string
	data, err := os.ReadFile(s.path("baselines", commit+"."+extractor+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return m, true, json.Unmarshal(data, &m)
}

func (s *Files) SaveSnapshot(commit, extractor string, pages map[string]string) error {
	data, err := json.Marshal(pages)
	if err != nil {
		return err
	}
	return writeFile(s.path("baselines", commit+"."+extractor+".json"), data)
}

// --- helpers ---

func readYAML(p string, into any) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	return nil
}

func writeYAML(p string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return writeFile(p, data)
}

// writeFile writes through a temporary file so a crash never leaves half a file.
func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
