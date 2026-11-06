// Package registry turns .raisq/ into the plugins a flow uses: one-of slots
// by this machine's config, the rest by every file with that slot.
package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/plugin/agent"
	"github.com/kkurno/raisq/plugin/builtin"
	"github.com/kkurno/raisq/plugin/command"
	"github.com/kkurno/raisq/plugin/detect"
	"github.com/kkurno/raisq/plugin/docker"
	"github.com/kkurno/raisq/plugin/git"
	"github.com/kkurno/raisq/plugin/skill"
	"github.com/kkurno/raisq/plugin/store"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/usecase"
)

// Config is this machine's settings, in .raisq/local/config.yaml: which agent
// and container tool to use here, and defaults to override.
type Config struct {
	// Preset is read only so an old raisq.yaml still loads; nothing uses it.
	Preset    any    `yaml:"preset,omitempty"`
	Builder   string `yaml:"builder"`
	Critic    string `yaml:"critic"`
	Isolation string `yaml:"isolation"`
	Attempts  int    `yaml:"attempts,omitempty"`
	Batch     string `yaml:"batch,omitempty"`
	SplitOver int    `yaml:"split_over,omitempty"`
	// SpotCheck is a pointer so that 0, no spot checks, differs from unset.
	SpotCheck *float64 `yaml:"spot_check,omitempty"`
	// KeepArtifacts is how long run folders are kept.
	KeepArtifacts string `yaml:"keep_artifacts,omitempty"`
}

type Project struct {
	Dir   string
	Raisq string
	// Moved is what Open moved from the layout before local/, to say once.
	Moved   []string
	Config  Config
	Files   []command.File
	Store   *store.Files
	Git     *git.Git
	WorkDir string
}

// ErrNoMachine means the project is set up, but this machine isn't yet, such
// as right after cloning it.
var ErrNoMachine = fmt.Errorf("this project uses RaisQ, but this machine isn't set up for it: run `raisq init`")

func Open(dir string) (*Project, error) {
	raisq := filepath.Join(dir, ".raisq")
	if _, err := os.Stat(raisq); err != nil {
		return nil, fmt.Errorf("no .raisq/ here: run `raisq init` in your project's folder")
	}
	moved, err := store.Migrate(raisq)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(ConfigPath(raisq))
	if err != nil {
		return nil, ErrNoMachine
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigPath(raisq), err)
	}
	files, err := command.Load(filepath.Join(raisq, "plugins"))
	if err != nil {
		return nil, err
	}
	work := filepath.Join(os.TempDir(), "raisq-work", filepath.Base(dir)+"-"+shortHash(dir))
	p := &Project{Dir: dir, Raisq: raisq, Moved: moved, Config: cfg, Files: files, Store: store.New(raisq), WorkDir: work}
	var private []string
	for _, s := range p.SkillFolders() {
		private = append(private, skill.Dir(s))
	}
	p.Git = git.New(dir, work, private...)
	return p, nil
}

// Agent is an agent's plugin file, the project's own or else the built-in one.
func (p *Project) Agent(name string) (command.File, bool) {
	for _, f := range p.Files {
		if f.Name == name && (f.Slot == "builder" || f.Slot == "model") {
			return f, true
		}
	}
	var f command.File
	if body, ok := detect.Builtin(name); ok && yaml.Unmarshal([]byte(body), &f) == nil && (f.Slot == "builder" || f.Slot == "model") {
		return f, true
	}
	return command.File{}, false
}

// SkillFolders is every folder RaisQ's skill may be written to: the standard
// one, and each known agent's own, so none of them reaches a builder.
func (p *Project) SkillFolders() []string {
	out := []string{skill.Standard}
	seen := map[string]bool{skill.Standard: true}
	add := func(f command.File) {
		if f.Skills != "" && !seen[f.Skills] {
			seen[f.Skills] = true
			out = append(out, f.Skills)
		}
	}
	for _, f := range p.Files {
		add(f)
	}
	for _, body := range detect.Builtins() {
		var f command.File
		if yaml.Unmarshal([]byte(body), &f) == nil {
			add(f)
		}
	}
	sort.Strings(out[1:])
	return out
}

// Person is the name to record on facts given here: git's user.name.
func (p *Project) Person() string {
	return p.Git.Person(context.Background())
}

// ConfigPath is this machine's settings file for a .raisq folder.
func ConfigPath(raisq string) string {
	return filepath.Join(raisq, store.Local, store.ConfigFile)
}

func shortHash(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return fmt.Sprintf("%08x", h)
}

func (p *Project) find(slotName, name string) (command.File, bool) {
	for _, f := range p.Files {
		if f.Slot == slotName && f.Name == name {
			return f, true
		}
	}
	return command.File{}, false
}

// HasChecks is whether any check judges the project's work.
func (p *Project) HasChecks() bool { return len(p.all("checks")) > 0 }

func (p *Project) all(slotName string) []command.File {
	var out []command.File
	for _, f := range p.Files {
		if f.Slot == slotName {
			out = append(out, f)
		}
	}
	return out
}

// Deps builds what a flow needs. task adds that task's own plugins.
func (p *Project) Deps(task string, manual bool) (usecase.Deps, error) {
	files := p.Files
	if task != "" {
		extra, err := command.Load(filepath.Join(p.Store.LocalDir(), "tasks", task, "plugins"))
		if err != nil {
			return usecase.Deps{}, err
		}
		files = append(append([]command.File(nil), files...), extra...)
	}
	saved := p.Files
	p.Files = files
	defer func() { p.Files = saved }()

	d := usecase.Deps{Store: p.Store, Checkout: p.Git, Manual: manual, Attempts: p.Config.Attempts, SplitOver: p.Config.SplitOver,
		Person: p.Person()}

	critic, ok := p.find("model", p.Config.Critic)
	if !ok {
		return d, fmt.Errorf("the critic is %q, and there's no plugin file with slot: model and that name. Choose one with raisq config critic <plugin>", p.Config.Critic)
	}
	d.Model = command.Model{F: critic}

	var builderFile command.File
	if !manual {
		builderFile, ok = p.find("builder", p.Config.Builder)
		if !ok {
			return d, fmt.Errorf("the builder is %q, and there's no plugin file with slot: builder and that name. Choose one with raisq config builder <plugin>", p.Config.Builder)
		}
		a := agent.Agent{PluginName: builderFile.Name, Run: builderFile.Run, Install: builderFile.Install}
		if builderFile.Tool != nil {
			a.Find = builderFile.Tool.Find
		}
		d.Builder = a
		switch {
		case critic.Vendor == "" || critic.Vendor != builderFile.Vendor:
		case critic.Model == builderFile.Model:
			d.Notices = append(d.Notices, fmt.Sprintf("The critic is the same model as the builder (%s %s), in its own session with no tools. It may share the builder's blind spots; a different model would catch more.", critic.Vendor, critic.Model))
		default:
			d.Notices = append(d.Notices, fmt.Sprintf("The critic (%s) is from the same vendor as the builder (%s): a different model, but closer blind spots than another vendor's.", critic.Model, builderFile.Model))
		}
	}
	if p.Config.Isolation == "none" {
		d.Notices = append(d.Notices, "isolation: none. The builder ran on this machine with only its own safeguards and the audit.")
	}

	iso, err := p.isolation(builderFile)
	if err != nil {
		return d, err
	}
	d.Isolation = iso

	for _, f := range p.all("checks") {
		d.Checks = append(d.Checks, p.check(f, task))
	}
	for _, f := range p.all("provides") {
		d.Providers = append(d.Providers, command.Provider{F: f, Project: p.Dir})
	}
	d.Extractors = []slot.Extractor{builtin.Diff{}}
	for _, f := range p.all("extractor") {
		switch f.Run {
		case "builtin:page-text":
			d.Extractors = append(d.Extractors, builtin.PageText{Port: f.Port})
		default:
			d.Extractors = append(d.Extractors, command.Extractor{F: f})
		}
	}
	d.Reports = []slot.Report{builtin.Markdown{Root: p.Store.LocalDir()}}
	for _, f := range p.all("report") {
		d.Reports = append(d.Reports, command.Report{F: f, Project: p.Dir})
	}
	for _, f := range p.all("spec-source") {
		d.Sources = append(d.Sources, command.Source{F: f, Project: p.Dir})
	}
	return d, nil
}

// check is the check a plugin file describes: a built-in one, or a command.
func (p *Project) check(f command.File, task string) slot.Check {
	switch f.Run {
	case "builtin:contacts":
		return builtin.Contacts{CheckName: f.Name, Facts: p.Store.LoadFacts, Pages: builtin.PageText{Port: f.Port}}
	case "builtin:go-api":
		return builtin.GoAPI{CheckName: f.Name, Store: p.Store, Base: func() string {
			st, _ := p.Store.LoadState(task)
			return st.Base
		}, Allowed: p.apiAllowed(task)}
	case "builtin:links":
		return builtin.Links{CheckName: f.Name, Pages: builtin.PageText{Port: f.Port}}
	}
	return command.Check{F: f}
}

func (p *Project) isolation(builder command.File) (slot.Isolation, error) {
	switch p.Config.Isolation {
	case "none":
		return Local{}, nil
	case "", "docker", "podman":
		bin := p.Config.Isolation
		if bin == "" {
			bin = "docker"
		}
		image := builder.Image
		if f, ok := p.find("isolation", bin); ok && f.Image != "" && image == "" {
			image = f.Image
		}
		if image == "" {
			image = "node:{node}-bookworm"
		}
		// {node} is the Node major the project asks for, so the container
		// runs what the project's own tooling expects.
		node := detect.NodeMajor(p.Dir)
		if node == "" {
			node = detect.DefaultNode
		}
		d := docker.New(strings.ReplaceAll(image, "{node}", node))
		d.Bin = bin
		d.LoginEnv = builder.LoginEnv
		for _, l := range builder.Login {
			if l.To == "" && l.Env == "" && l.File != "" {
				l.To = "/home/raisq/" + strings.TrimPrefix(l.File, "~/")
			}
			d.Login = append(d.Login, l)
		}
		return d, nil
	}
	return nil, fmt.Errorf("unknown isolation %q", p.Config.Isolation)
}

// Keep is how long run folders are kept, from keep_artifacts ("30d"), 30
// days by default.
func (p *Project) Keep() time.Duration {
	if d, err := store.ParseDays(p.Config.KeepArtifacts); err == nil && d > 0 {
		return d
	}
	return 30 * 24 * time.Hour
}

// SpotShare is the share of done tasks raisq learn picks to re-read, 0.2 by
// default.
func (p *Project) SpotShare() float64 {
	if p.Config.SpotCheck != nil {
		return *p.Config.SpotCheck
	}
	return 0.2
}

// BatchLimit is the batch time limit, 8 hours by default.
func (p *Project) BatchLimit() time.Duration {
	if d, err := time.ParseDuration(p.Config.Batch); err == nil && d > 0 {
		return d
	}
	return 8 * time.Hour
}

// apiAllowed is whether the task spec says the public API may change.
func (p *Project) apiAllowed(task string) bool {
	if task == "" {
		return false
	}
	spec, err := p.Store.LoadSpec(task)
	return err == nil && spec.ChangesAPI
}
