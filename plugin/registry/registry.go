// Package registry turns .raisq/ into the plugins a flow uses, by the rule in
// 6.2: one-of slots by raisq.yaml, the rest by every file with that slot.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/plugin/agent"
	"github.com/kkurno/raisq/plugin/builtin"
	"github.com/kkurno/raisq/plugin/command"
	"github.com/kkurno/raisq/plugin/docker"
	"github.com/kkurno/raisq/plugin/git"
	"github.com/kkurno/raisq/plugin/store"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/usecase"
)

// Config is raisq.yaml: one-of slot choices, and defaults a project overrides.
type Config struct {
	Preset    any     `yaml:"preset,omitempty"`
	Builder   string  `yaml:"builder"`
	Critic    string  `yaml:"critic"`
	Isolation string  `yaml:"isolation"`
	Attempts  int     `yaml:"attempts,omitempty"`
	Batch     string  `yaml:"batch,omitempty"`
	SplitOver int     `yaml:"split_over,omitempty"`
	SpotCheck float64 `yaml:"spot_check,omitempty"`
	// KeepArtifacts is how long run folders are kept.
	KeepArtifacts string `yaml:"keep_artifacts,omitempty"`
}

type Project struct {
	Dir     string
	Raisq   string
	Config  Config
	Files   []command.File
	Store   *store.Files
	Git     *git.Git
	WorkDir string
}

func Open(dir string) (*Project, error) {
	raisq := filepath.Join(dir, ".raisq")
	data, err := os.ReadFile(filepath.Join(raisq, "raisq.yaml"))
	if err != nil {
		return nil, fmt.Errorf("no .raisq/raisq.yaml here: run `raisq init` first")
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("raisq.yaml: %w", err)
	}
	files, err := command.Load(filepath.Join(raisq, "plugins"))
	if err != nil {
		return nil, err
	}
	work := filepath.Join(os.TempDir(), "raisq-work", filepath.Base(dir)+"-"+shortHash(dir))
	return &Project{Dir: dir, Raisq: raisq, Config: cfg, Files: files,
		Store: store.New(raisq), Git: git.New(dir, work), WorkDir: work}, nil
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
		extra, err := command.Load(filepath.Join(p.Raisq, "tasks", task, "plugins"))
		if err != nil {
			return usecase.Deps{}, err
		}
		files = append(append([]command.File(nil), files...), extra...)
	}
	saved := p.Files
	p.Files = files
	defer func() { p.Files = saved }()

	d := usecase.Deps{Store: p.Store, Checkout: p.Git, Manual: manual, Attempts: p.Config.Attempts, SplitOver: p.Config.SplitOver}

	critic, ok := p.find("model", p.Config.Critic)
	if !ok {
		return d, fmt.Errorf("raisq.yaml names critic %q, and there's no plugin file with slot: model and that name", p.Config.Critic)
	}
	d.Model = command.Model{F: critic}

	var builderFile command.File
	if !manual {
		builderFile, ok = p.find("builder", p.Config.Builder)
		if !ok {
			return d, fmt.Errorf("raisq.yaml names builder %q, and there's no plugin file with slot: builder and that name", p.Config.Builder)
		}
		if critic.Vendor != "" && critic.Vendor == builderFile.Vendor && critic.Model == builderFile.Model {
			return d, fmt.Errorf("the critic must be a different model from the builder, or it shares its blind spots: both are %s %s", critic.Vendor, critic.Model)
		}
		a := agent.Agent{PluginName: builderFile.Name, Run: builderFile.Run, Install: builderFile.Install}
		if builderFile.Tool != nil {
			a.Find = builderFile.Tool.Find
		}
		d.Builder = a
		if critic.Vendor != "" && critic.Vendor == builderFile.Vendor {
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
		switch f.Run {
		case "builtin:contacts":
			d.Checks = append(d.Checks, builtin.Contacts{CheckName: f.Name, Facts: p.Store.LoadFacts, Pages: builtin.PageText{Port: f.Port}})
		case "builtin:go-api":
			d.Checks = append(d.Checks, builtin.GoAPI{CheckName: f.Name, Store: p.Store, Base: func() string {
				st, _ := p.Store.LoadState(task)
				return st.Base
			}, Allowed: p.apiAllowed(task)})
		case "builtin:links":
			d.Checks = append(d.Checks, builtin.Links{CheckName: f.Name, Pages: builtin.PageText{Port: f.Port}})
		default:
			d.Checks = append(d.Checks, command.Check{F: f})
		}
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
	d.Reports = []slot.Report{builtin.Markdown{Root: p.Raisq}}
	for _, f := range p.all("report") {
		d.Reports = append(d.Reports, command.Report{F: f, Project: p.Dir})
	}
	for _, f := range p.all("spec-source") {
		d.Sources = append(d.Sources, command.Source{F: f, Project: p.Dir})
	}
	for _, f := range files {
		if s := f.RenderForAI(); s != "" {
			d.ForAI = append(d.ForAI, s)
		}
	}
	return d, nil
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
			image = "node:22-bookworm"
		}
		d := docker.New(image)
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

// BatchLimit is the batch time limit from raisq.yaml, 8 hours by default.
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
