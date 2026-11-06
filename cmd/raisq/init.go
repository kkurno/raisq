package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/plugin/detect"
	"github.com/kkurno/raisq/plugin/registry"
	"github.com/kkurno/raisq/plugin/skill"
	"github.com/kkurno/raisq/plugin/store"
)

// cmdInit sets up RaisQ here, and is safe to run again: it fills in whatever
// is missing, such as this machine's settings after cloning, or the skill
// after an update, and never changes what's already set.
func cmdInit(ctx context.Context, args []string) error {
	fs := flags("init")
	yes := fs.Bool("yes", false, "accept what init proposes without asking")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	dir := wd()
	raisq := filepath.Join(dir, ".raisq")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		ok, err := confirm(*yes, "This isn't a git repo. Run `git init` and commit what's here?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("RaisQ needs git")
		}
		for _, c := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-qm", "Before RaisQ"}} {
			cmd := exec.Command("git", c...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git %s: %s", c[0], out)
			}
		}
	}
	if _, err := os.Stat(raisq); err == nil {
		if moved, err := store.Migrate(raisq); err != nil {
			return err
		} else if len(moved) > 0 {
			logf("Moved this machine's RaisQ state into .raisq/local/: %s.", strings.Join(moved, ", "))
		}
	}

	// This machine's settings: kept if they're there, chosen from what's
	// installed if not.
	var cfg registry.Config
	if data, err := os.ReadFile(registry.ConfigPath(raisq)); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("%s: %w. Fix it by hand, or delete it and run raisq init again", registry.ConfigPath(raisq), err)
		}
		logf("This machine: builder %s, critic %s, isolation %s. raisq config changes them.", cfg.Builder, cfg.Critic, orDefault(cfg.Isolation, "docker"))
	} else {
		m := detect.DetectMachine()
		logf("Found: %s", strings.Join(m.Found, " · "))
		logf("Builder: %s   Critic: %s   Isolation: %s", m.Builder, m.Critic, m.Isolation)
		for _, n := range m.Notes {
			logf("note: %s", n)
		}
		ok, err := confirm(*yes, "Use these on this machine? You can change any of it afterwards")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("nothing written")
		}
		if err := writeMachine(raisq, m.Builder, m.Critic, m.Isolation); err != nil {
			return err
		}
		cfg = registry.Config{Builder: m.Builder, Critic: m.Critic, Isolation: m.Isolation}
	}

	// The project's side: folders, the chosen agents' plugin files, the skill.
	// Each is added if it's missing; nothing there is replaced.
	var added []string
	for _, d := range []string{"facts", "plugins", "rubrics"} {
		if err := os.MkdirAll(filepath.Join(raisq, d), 0o755); err != nil {
			return err
		}
	}
	for _, name := range []string{cfg.Builder, cfg.Critic} {
		p := filepath.Join(raisq, "plugins", name+".yaml")
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if body, ok := detect.Builtin(name); ok {
			if err := write(p, body); err != nil {
				return err
			}
			added = append(added, filepath.Join(".raisq", "plugins", name+".yaml"))
		}
	}
	proj, err := registry.Open(dir)
	if err != nil {
		return err
	}
	// The skill goes in the standard folder and any other a copy was put in,
	// even an emptied one; a missing or old copy is written again.
	folders := []string{skill.Standard}
	for _, f := range proj.SkillFolders() {
		if _, err := os.Stat(filepath.Join(dir, skill.Dir(f))); err == nil {
			folders = append(folders, f)
		}
	}
	stale := map[string]bool{}
	for _, s := range skill.Stale(dir, folders) {
		stale[s] = true
	}
	var refresh []string
	for _, f := range unique(folders) {
		if _, err := os.Stat(filepath.Join(dir, skill.Dir(f), "SKILL.md")); err != nil || stale[skill.Dir(f)] {
			refresh = append(refresh, f)
		}
	}
	if len(refresh) > 0 {
		if _, err := skill.Write(dir, refresh); err != nil {
			return err
		}
		for _, f := range refresh {
			added = append(added, skill.Dir(f))
		}
	}
	for _, a := range added {
		logf("+ %s", a)
	}

	switch {
	case !proj.HasChecks():
		logf("Ready. RaisQ knows your agent and container tool, but not yet how to check this project.")
		logf("Next, in your agent's chat: /raisq set up this project")
		// The agent you chat in may read skills from its own folder.
		if f, ok := proj.Agent(cfg.Builder); ok && f.Skills != "" && len(skill.Written(dir, []string{f.Skills})) == 0 {
			logf("If you chat in %s, it reads skills from %s, so first: raisq skill %s", f.Name, f.Skills, f.Name)
		}
	case len(added) == 0:
		logf("Everything is in place. raisq doctor checks the tools; raisq status shows the tasks.")
	default:
		logf("Ready.")
	}
	if len(added) > 0 {
		logf("Commit .raisq/ and .agents/: they're the project's setup. .raisq/local/ is yours, and git ignores it.")
	}
	return nil
}

// confirm asks a person at the terminal, or takes --yes. With neither, it
// says how to run it without asking.
func confirm(yes bool, q string) (bool, error) {
	if yes {
		return true, nil
	}
	t := person()
	if t == nil {
		return false, fmt.Errorf("nothing written: there's no terminal to ask. To accept: raisq init --yes")
	}
	return t.Confirm(q)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// writeMachine writes this machine's settings and the folder they live in.
func writeMachine(raisq, builder, critic, isolation string) error {
	if err := store.EnsureLocal(raisq); err != nil {
		return err
	}
	data, err := yaml.Marshal(registry.Config{Builder: builder, Critic: critic, Isolation: isolation})
	if err != nil {
		return err
	}
	return write(registry.ConfigPath(raisq), "# This machine's RaisQ settings. Not committed: each person has their own.\n"+string(data))
}

func write(p, body string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(body), 0o644)
}
