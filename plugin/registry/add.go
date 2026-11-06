package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/command"
	"github.com/kkurno/raisq/plugin/detect"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/usecase"
)

// Candidate is a plugin about to be added: what it is, whether it fills its
// slot's contract, and, for a check, how it did on the project as it is.
type Candidate struct {
	File     command.File
	Body     string
	BuiltIn  bool
	Problems []string
	// Warnings don't stop it being added, but a person should know them.
	Warnings []string
	Trial    *usecase.Trial
}

// Protects is what a check keeps the builder from changing: its tests and
// locks, relative to the project.
func (c Candidate) Protects() []string {
	if c.File.Slot != "checks" {
		return nil
	}
	ch := command.Check{F: c.File}
	return append(ch.Tests(), ch.Locks()...)
}

// warnings says what a check leaves unprotected. RaisQ can't tell from a
// command whether it runs tests, so it says what's missing and lets a person
// decide; a build or a lint is fine without either.
func warnings(f command.File) []string {
	if f.Slot != "checks" || strings.HasPrefix(f.Run, "builtin:") {
		return nil
	}
	var w []string
	if len(f.Tests) == 0 && len(f.Locks) == 0 {
		w = append(w, "it lists no tests: or locks:, so nothing it uses is put back before it runs. If it runs tests or reads a config, the builder could edit those to pass")
	}
	if len(f.Tests) > 0 && f.RunOne == "" {
		w = append(w, "it has no run_one, so tests the builder adds can't be proven to test anything")
	}
	return w
}

// Ready is whether it can be added: it fills its slot's contract.
func (c Candidate) Ready() bool { return len(c.Problems) == 0 }

// Builtin is one of RaisQ's own plugin files.
type Builtin struct {
	Name, Slot, What string
}

// Builtins is RaisQ's library of plugin files, to add as they are.
func Builtins() []Builtin {
	var out []Builtin
	for name, body := range detect.Builtins() {
		var f command.File
		if yaml.Unmarshal([]byte(body), &f) != nil || f.Slot == "" {
			continue
		}
		out = append(out, Builtin{Name: name, Slot: f.Slot, What: f.Description})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Prepare reads what's to be added, plugin files' paths or built-ins' names,
// and tests each: against its slot's contract, then every check once on the
// project as it is, together in one fresh container with what they need. A
// provider among them is available to the checks with it. It saves nothing.
func (p *Project) Prepare(ctx context.Context, from ...string) ([]Candidate, error) {
	var cs []Candidate
	names := map[string]bool{}
	for _, src := range from {
		c := Candidate{}
		if data, err := os.ReadFile(src); err == nil {
			c.Body = string(data)
		} else if body, ok := detect.Builtin(src); ok {
			c.Body, c.BuiltIn = body, true
		} else {
			return nil, fmt.Errorf("%s isn't a file or one of RaisQ's plugins: raisq plugin list --built-in lists those", src)
		}
		if err := yaml.Unmarshal([]byte(c.Body), &c.File); err != nil {
			return nil, fmt.Errorf("%s isn't a plugin file: %w", src, err)
		}
		if f, ok := p.byName(c.File.Name); ok {
			return nil, fmt.Errorf("the project already has a plugin named %s (%s): remove it first, or give this one another name", c.File.Name, filepath.Base(f.Path))
		}
		if names[c.File.Name] {
			return nil, fmt.Errorf("two of these are named %s: give each its own name", c.File.Name)
		}
		names[c.File.Name] = true
		c.Problems = command.Contract(ctx, c.Body, p.Dir)
		c.Warnings = warnings(c.File)
		cs = append(cs, c)
	}
	// A check's providers come from the project or from what's added with it.
	provided := map[string]command.File{}
	for _, f := range p.all("provides") {
		provided[f.Name] = f
	}
	for _, c := range cs {
		if c.File.Slot == "provides" && c.Ready() {
			provided[c.File.Name] = c.File
		}
	}
	var checks []slot.Check
	for i := range cs {
		c := &cs[i]
		if c.File.Slot != "checks" || !c.Ready() {
			continue
		}
		for _, n := range c.File.Needs {
			if _, ok := provided[n]; !ok {
				c.Problems = append(c.Problems, fmt.Sprintf("it needs %s, and there's no provider by that name: add one with it", n))
			}
		}
		if c.Ready() {
			checks = append(checks, p.check(c.File, ""))
		}
	}
	if len(checks) == 0 {
		return cs, nil
	}
	d, err := p.Deps("", true)
	if err != nil {
		return nil, err
	}
	d.Providers = nil
	for _, f := range provided {
		d.Providers = append(d.Providers, command.Provider{F: f, Project: p.Dir})
	}
	if err := d.Isolation.Available(ctx); err != nil {
		for i := range cs {
			if cs[i].File.Slot == "checks" && cs[i].Ready() {
				cs[i].Problems = append(cs[i].Problems, "can't try it: "+err.Error())
			}
		}
		return cs, nil
	}
	trials, err := usecase.TrialChecks(ctx, d, checks)
	if err != nil {
		return nil, err
	}
	for i := range cs {
		if t, ok := trials[cs[i].File.Name]; ok {
			cs[i].Trial = &t
		}
	}
	return cs, nil
}

// TryCheck runs one of the project's checks once on the project as it is, in
// a fresh container with the providers it needs, the same trial plugin add
// runs before adding one.
func (p *Project) TryCheck(ctx context.Context, name string) (usecase.Trial, error) {
	f, ok := p.find("checks", name)
	if !ok {
		return usecase.Trial{}, fmt.Errorf("no check %s in .raisq/plugins/", name)
	}
	d, err := p.Deps("", true)
	if err != nil {
		return usecase.Trial{}, err
	}
	if err := d.Isolation.Available(ctx); err != nil {
		return usecase.Trial{}, err
	}
	trials, err := usecase.TrialChecks(ctx, d, []slot.Check{p.check(f, "")})
	if err != nil {
		return usecase.Trial{}, err
	}
	return trials[name], nil
}

// Add saves a prepared plugin into .raisq/plugins/, never over one that's
// there, and returns where.
func (p *Project) Add(c Candidate) (string, error) {
	if !c.Ready() {
		return "", fmt.Errorf("%s isn't added: %s", c.File.Name, strings.Join(c.Problems, "; "))
	}
	path := filepath.Join(p.Raisq, "plugins", c.File.Name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s exists: not overwritten", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(c.Body), 0o644); err != nil {
		return "", err
	}
	c.File.Path = path
	p.Files = append(p.Files, c.File)
	rel, _ := filepath.Rel(p.Dir, path)
	return rel, nil
}

func (p *Project) byName(name string) (command.File, bool) {
	for _, f := range p.Files {
		if f.Name == name {
			return f, true
		}
	}
	return command.File{}, false
}

// AddRubrics adds the rubrics in a file, or a built-in set such as web, to
// rubrics/, each checked first. It never replaces one with the same id.
func (p *Project) AddRubrics(from string) (string, []domain.Rubric, error) {
	data, err := os.ReadFile(from)
	name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(from), filepath.Ext(from)), "-rubrics")
	if err != nil {
		body, ok := detect.Builtin(from + "-rubrics")
		if !ok {
			return "", nil, fmt.Errorf("%s isn't a file or one of RaisQ's rubric sets: raisq rubric list --built-in lists those", from)
		}
		data, name = []byte(body), from
	}
	var rs []domain.Rubric
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return "", nil, fmt.Errorf("%s isn't a list of rubrics: %w", from, err)
	}
	if len(rs) == 0 {
		return "", nil, fmt.Errorf("%s has no rubrics", from)
	}
	have, err := p.Store.LoadRubrics()
	if err != nil {
		return "", nil, err
	}
	ids := map[string]bool{}
	for _, r := range have {
		ids[r.ID] = true
	}
	for _, r := range rs {
		if err := r.Validate(); err != nil {
			return "", nil, err
		}
		if ids[r.ID] {
			return "", nil, fmt.Errorf("the project already has a rubric %s: give this one another id", r.ID)
		}
		ids[r.ID] = true
	}
	for _, r := range rs {
		if err := p.Store.SaveRubric(r, name); err != nil {
			return "", nil, err
		}
	}
	return filepath.Join(".raisq", "rubrics", name+".yaml"), rs, nil
}

// RubricSets is RaisQ's built-in rubric sets, by name.
func RubricSets() []string {
	var out []string
	for name := range detect.Builtins() {
		if set, ok := strings.CutSuffix(name, "-rubrics"); ok {
			out = append(out, set)
		}
	}
	sort.Strings(out)
	return out
}
