package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/detect"
	"github.com/kkurno/raisq/plugin/registry"
	"github.com/kkurno/raisq/plugin/skill"
	"github.com/kkurno/raisq/plugin/store"
)

func cmdInit(ctx context.Context, args []string) error {
	fs := flags("init")
	yes := fs.Bool("yes", false, "accept what init proposes without asking")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	dir := wd()
	raisq := filepath.Join(dir, ".raisq")
	if _, err := os.Stat(raisq); err == nil {
		return initMachine(raisq, *yes)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t := newTTY()
		ok := *yes
		if !ok {
			var err error
			if ok, err = t.Confirm("This isn't a git repo. Run `git init` and commit what's here?"); err != nil {
				return err
			}
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

	p := detect.Detect(dir)
	fmt.Printf("  Found: %s\n", strings.Join(p.Found, " · "))
	fmt.Printf("  Preset: %s   Builder: %s   Critic: %s   Isolation: %s\n",
		strings.Join(p.Presets, ", "), p.Builder, p.Critic, p.Isolation)
	var names []string
	for n := range p.Files {
		names = append(names, strings.TrimSuffix(n, ".yaml"))
	}
	sort.Strings(names)
	fmt.Printf("  Plugin files: %s\n", strings.Join(names, ", "))
	for _, n := range p.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	if !*yes {
		ok, err := newTTY().Confirm("Write these into .raisq/? You can change any of it afterwards")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("nothing written")
		}
	}

	for n, body := range p.Files {
		if err := write(filepath.Join(raisq, "plugins", n), body); err != nil {
			return err
		}
	}
	for n, body := range p.Rubrics {
		if err := write(filepath.Join(raisq, "rubrics", n+".yaml"), body); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(raisq, "facts"), 0o755); err != nil {
		return err
	}
	if err := writeMachine(raisq, p.Builder, p.Critic, p.Isolation); err != nil {
		return err
	}
	if contains(p.Presets, "web") || contains(p.Presets, "docs") {
		if err := seedFacts(dir, *yes); err != nil {
			return err
		}
	}
	if _, err := skill.Write(dir, []string{skill.Standard}); err != nil {
		return err
	}
	fmt.Println("  Ready. Commit .raisq/ and .agents/: they're the project's setup, shared with your team.")
	fmt.Println("  .raisq/local/ is this machine's: your settings, tasks and runs. Git ignores it.")
	fmt.Println("  To change what was chosen: raisq config, and raisq plugin add or remove.")
	fmt.Println("  Next: raisq task \"what you want built\"")
	fmt.Printf("  Or from your agent's chat: /raisq what you want. For an agent that doesn't read %s, run raisq skill <agent>, such as raisq skill claude-code.\n", skill.Standard)
	return nil
}

// initMachine sets up this machine for a project that's already set up, such
// as one just cloned: it picks the agents and container tool found here, and
// leaves the project's shared files alone.
func initMachine(raisq string, yes bool) error {
	if _, err := store.Migrate(raisq); err != nil {
		return err
	}
	if _, err := os.Stat(registry.ConfigPath(raisq)); err == nil {
		return fmt.Errorf("this machine is already set up: raisq config shows its settings")
	}
	m := detect.DetectMachine()
	fmt.Printf("  This project already uses RaisQ. Setting up this machine.\n")
	fmt.Printf("  Found: %s\n", strings.Join(m.Found, " · "))
	fmt.Printf("  Builder: %s   Critic: %s   Isolation: %s\n", m.Builder, m.Critic, m.Isolation)
	for _, n := range m.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	// An agent's plugin file the project lacks is added, never one replaced.
	var added []string
	for n := range m.Files {
		if _, err := os.Stat(filepath.Join(raisq, "plugins", n)); err != nil {
			added = append(added, n)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		fmt.Printf("  Adds to the project's plugins: %s\n", strings.Join(added, ", "))
	}
	if !yes {
		ok, err := newTTY().Confirm("Use these on this machine? You can change any of it afterwards")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("nothing written")
		}
	}
	for _, n := range added {
		if err := write(filepath.Join(raisq, "plugins", n), m.Files[n]); err != nil {
			return err
		}
	}
	if err := writeMachine(raisq, m.Builder, m.Critic, m.Isolation); err != nil {
		return err
	}
	fmt.Println("  Ready. raisq config changes this machine's settings; your teammates keep theirs.")
	return nil
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

// seedFacts asks the questions a site's facts need, once.
func seedFacts(dir string, skip bool) error {
	if skip {
		return nil
	}
	proj, err := registry.Open(dir)
	if err != nil {
		return err
	}
	t := newTTY()
	fmt.Println("  A few facts about the project. Leave one empty if it's unknown; it stays out of the output.")
	today := time.Now().Format("2006-01-02")
	who := proj.Person()
	if who == "" {
		who = "you"
	}
	for _, q := range []struct{ id, kind, ask string }{
		{"business/name", "terms", "Business or product name?"},
		{"business/phone", "contact", "Phone number shown to users?"},
		{"business/email", "contact", "Email shown to users?"},
		{"business/address", "contact", "Address shown to users?"},
	} {
		a, err := t.Ask(q.ask)
		if err != nil {
			return err
		}
		f := domain.Fact{ID: q.id, Kind: q.kind, Tag: domain.Gap}
		if a != "" {
			f = domain.Fact{ID: q.id, Text: a, Kind: q.kind, Tag: domain.Confirmed, SaidBy: who, SaidOn: today}
		}
		if err := proj.Store.SaveFact(f, "business"); err != nil {
			return err
		}
	}
	return nil
}

func write(p, body string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(body), 0o644)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
