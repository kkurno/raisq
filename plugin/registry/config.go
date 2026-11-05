package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/command"
	"github.com/kkurno/raisq/plugin/detect"
	"github.com/kkurno/raisq/plugin/store"
)

// Setting is one line of raisq.yaml as raisq config shows it.
type Setting struct {
	Key, Value string
	Default    bool
}

// Settings is every key raisq config can change, with defaults filled in.
func (p *Project) Settings() []Setting {
	c := p.Config
	def := func(key, value, fallback string) Setting {
		if value == "" || value == "0" {
			return Setting{key, fallback, true}
		}
		return Setting{key, value, false}
	}
	spot := Setting{"spot_check", "0.2", true}
	if c.SpotCheck != nil {
		spot = Setting{"spot_check", strconv.FormatFloat(*c.SpotCheck, 'f', -1, 64), false}
	}
	out := []Setting{
		{"builder", c.Builder, false},
		{"critic", c.Critic, false},
		def("isolation", c.Isolation, "docker"),
		def("attempts", strconv.Itoa(c.Attempts), "3"),
		def("batch", c.Batch, "8h"),
		def("split_over", strconv.Itoa(c.SplitOver), "6"),
		spot,
		def("keep_artifacts", c.KeepArtifacts, "30d"),
	}
	if c.Preset != nil {
		out = append(out, Setting{"preset", fmt.Sprint(c.Preset), false})
	}
	return out
}

var keys = []string{"builder", "critic", "isolation", "attempts", "batch", "split_over", "spot_check", "keep_artifacts"}

// ErrConfirm means the change needs a person to agree to it at a terminal.
type ErrConfirm struct{ Warning string }

func (e ErrConfirm) Error() string { return e.Warning }

// Set checks a value and saves it to raisq.yaml, leaving the rest of the file,
// comments included, as it was. confirmed is a person's yes to an ErrConfirm.
// The notes say what else changed or what to know.
func (p *Project) Set(key, value string, confirmed bool) ([]string, error) {
	key = strings.ReplaceAll(key, "-", "_")
	value = strings.TrimSpace(value)
	var typed any = value
	var notes []string
	switch key {
	case "builder", "critic":
		slotName := map[string]string{"builder": "builder", "critic": "model"}[key]
		wrote, err := p.ensurePlugin(slotName, value)
		if err != nil {
			return nil, err
		}
		if wrote != "" {
			notes = append(notes, "Added "+wrote+", from RaisQ's built-in plugins. It's yours to edit and commit.")
		}
	case "isolation":
		switch value {
		case "docker", "podman":
		case "none":
			if !confirmed {
				return nil, ErrConfirm{"isolation: none runs the builder directly on this machine, with its permission prompts off and nothing between it and your files, keys and logins. Only use it on a machine you're happy to throw away, such as a CI runner. Every review pack will say so."}
			}
		default:
			return nil, fmt.Errorf("isolation is docker, podman or none, not %q", value)
		}
	case "attempts", "split_over":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%s is a whole number, 1 or more, not %q", key, value)
		}
		typed = n
	case "batch":
		d, err := time.ParseDuration(value)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("batch is a length of time, such as 6h or 90m, not %q", value)
		}
	case "spot_check":
		f, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err == nil && strings.HasSuffix(value, "%") {
			f /= 100
		}
		if err != nil || f < 0 || f > 1 {
			return nil, fmt.Errorf("spot_check is the share of done tasks to re-read, from 0 to 1, such as 0.2 or 20%%, not %q", value)
		}
		typed = f
	case "keep_artifacts":
		if d, err := store.ParseDays(value); err != nil || d <= 0 {
			return nil, fmt.Errorf("keep_artifacts is a number of days or weeks, such as 30d or 4w, not %q", value)
		}
	case "preset":
		return nil, fmt.Errorf("preset records what init found, and changing it changes nothing: add or remove checks with raisq plugin new, add and remove")
	default:
		return nil, fmt.Errorf("no setting %q. Settings: %s", key, strings.Join(keys, ", "))
	}
	if err := p.edit(key, typed); err != nil {
		return nil, err
	}
	if key == "builder" || key == "critic" {
		if n := p.sameModel(); n != "" {
			notes = append(notes, n)
		}
	}
	return append(notes, p.runningNote()...), nil
}

// Unset puts a key back to its default.
func (p *Project) Unset(key string) ([]string, error) {
	key = strings.ReplaceAll(key, "-", "_")
	switch key {
	case "builder", "critic":
		return nil, fmt.Errorf("%s has no default: choose another with raisq config %s <plugin>", key, key)
	case "isolation", "attempts", "batch", "split_over", "spot_check", "keep_artifacts":
	default:
		return nil, fmt.Errorf("no setting %q. Settings: %s", key, strings.Join(keys, ", "))
	}
	if err := p.edit(key, nil); err != nil {
		return nil, err
	}
	return p.runningNote(), nil
}

// edit sets key to value in raisq.yaml, or removes it when value is nil, then
// reloads the config.
func (p *Project) edit(key string, value any) error {
	path := filepath.Join(p.Raisq, "raisq.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w. Fix it by hand, then try again", path, err)
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return fmt.Errorf("%s isn't a list of settings: fix it by hand, then try again", path)
	}
	at := -1
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			at = i
		}
	}
	switch {
	case value == nil && at >= 0:
		m.Content = append(m.Content[:at], m.Content[at+2:]...)
	case value != nil:
		var v yaml.Node
		if err := v.Encode(value); err != nil {
			return err
		}
		if at >= 0 {
			v.LineComment = m.Content[at+1].LineComment
			*m.Content[at+1] = v
		} else {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &v)
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	var cfg Config
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		return err
	}
	// A half-written raisq.yaml would stop every command, so swap it in whole.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	p.Config = cfg
	return nil
}

// ensurePlugin makes sure a plugin file for slot and name is in the project:
// the project's own, or else RaisQ's built-in one, copied in. It never
// overwrites a file the project has.
func (p *Project) ensurePlugin(slotName, name string) (string, error) {
	if _, ok := p.find(slotName, name); ok {
		return "", nil
	}
	for _, f := range p.Files {
		if f.Name == name {
			return "", fmt.Errorf("%s is a %s plugin, not a %s one", name, f.Slot, slotName)
		}
	}
	body, ok := detect.Builtin(name)
	var f command.File
	if !ok || yaml.Unmarshal([]byte(body), &f) != nil || f.Slot != slotName {
		return "", fmt.Errorf("no %s plugin %q. Choose one of: %s, or add one with raisq plugin new %s %s",
			slotName, name, strings.Join(p.choices(slotName), ", "), slotName, name)
	}
	path := filepath.Join(p.Raisq, "plugins", name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s exists but isn't a %s plugin named %s: fix or rename it first", path, slotName, name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	f.Path = path
	p.Files = append(p.Files, f)
	rel, _ := filepath.Rel(p.Dir, path)
	return rel, nil
}

// choices is every plugin name that can fill slot: the project's, then the
// built-in ones.
func (p *Project) choices(slotName string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range p.all(slotName) {
		seen[f.Name] = true
		out = append(out, f.Name)
	}
	var built []string
	for name, body := range detect.Builtins() {
		var f command.File
		if !seen[name] && yaml.Unmarshal([]byte(body), &f) == nil && f.Slot == slotName {
			built = append(built, name)
		}
	}
	sort.Strings(built)
	return append(out, built...)
}

func (p *Project) sameModel() string {
	b, okB := p.find("builder", p.Config.Builder)
	c, okC := p.find("model", p.Config.Critic)
	if !okB || !okC || b.Vendor == "" || b.Vendor != c.Vendor || b.Model != c.Model {
		return ""
	}
	return fmt.Sprintf("The critic is the same model as the builder (%s %s). That's allowed, and every review pack will say so, but a different model catches more.", b.Vendor, b.Model)
}

func (p *Project) runningNote() []string {
	names, _ := p.Store.Tasks()
	for _, n := range names {
		if st, err := p.Store.LoadState(n); err == nil && st.State == domain.Running {
			return []string{"A task is running. It keeps the settings it started with; the change applies from the next run."}
		}
	}
	return nil
}

// RemovePlugin deletes a plugin file from .raisq/plugins/, unless something
// still depends on it.
func (p *Project) RemovePlugin(name string) (string, []string, error) {
	var target command.File
	found := false
	for _, f := range p.Files {
		if f.Name == name {
			target, found = f, true
		}
	}
	if !found {
		var names []string
		for _, f := range p.Files {
			names = append(names, f.Name)
		}
		return "", nil, fmt.Errorf("no plugin %s in .raisq/plugins/. Plugins: %s", name, strings.Join(names, ", "))
	}
	switch {
	case target.Slot == "builder" && name == p.Config.Builder:
		return "", nil, fmt.Errorf("%s is the builder: choose another first, with raisq config builder <plugin>", name)
	case target.Slot == "model" && name == p.Config.Critic:
		return "", nil, fmt.Errorf("%s is the critic: choose another first, with raisq config critic <plugin>", name)
	case target.Slot == "isolation" && name == p.Settings()[2].Value:
		return "", nil, fmt.Errorf("%s is the isolation in use: choose another first, with raisq config isolation <docker|podman>", name)
	case target.Slot == "provides":
		var users []string
		for _, f := range p.Files {
			for _, n := range f.Needs {
				if n == name {
					users = append(users, f.Name)
				}
			}
		}
		if len(users) > 0 {
			return "", nil, fmt.Errorf("%s needs %s, and would block every task without it: remove those first", strings.Join(users, ", "), name)
		}
	}
	if err := os.Remove(target.Path); err != nil {
		return "", nil, err
	}
	kept := p.Files[:0]
	for _, f := range p.Files {
		if f.Path != target.Path {
			kept = append(kept, f)
		}
	}
	p.Files = kept
	var notes []string
	if target.Lesson != "" {
		notes = append(notes, fmt.Sprintf("It was a lesson, learned from %s. The mistake it caught won't be checked any more.", strings.TrimPrefix(target.Lesson, "task/")))
	}
	if target.Slot == "checks" && len(p.all("checks")) == 0 {
		notes = append(notes, "No checks are left, so only the critic judges the work. Add one with raisq plugin new checks <name>.")
	}
	rel, _ := filepath.Rel(p.Dir, target.Path)
	return rel, append(notes, p.runningNote()...), nil
}
