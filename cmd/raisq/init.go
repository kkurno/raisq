package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

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

	m := detect.DetectMachine()
	fmt.Printf("  Found: %s\n", strings.Join(m.Found, " · "))
	fmt.Printf("  Builder: %s   Critic: %s   Isolation: %s\n", m.Builder, m.Critic, m.Isolation)
	for _, n := range m.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	if !*yes {
		ok, err := newTTY().Confirm("Set up RaisQ here with these? You can change any of it afterwards")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("nothing written")
		}
	}
	for n, body := range m.Files {
		if err := write(filepath.Join(raisq, "plugins", n), body); err != nil {
			return err
		}
	}
	for _, d := range []string{"facts", "rubrics"} {
		if err := os.MkdirAll(filepath.Join(raisq, d), 0o755); err != nil {
			return err
		}
	}
	if err := writeMachine(raisq, m.Builder, m.Critic, m.Isolation); err != nil {
		return err
	}
	if _, err := skill.Write(dir, []string{skill.Standard}); err != nil {
		return err
	}
	fmt.Println("  Ready. RaisQ knows your agent and container tool, but not yet how to check this project.")
	fmt.Println("  Next, in your agent's chat: /raisq set up this project")
	fmt.Println("  It reads your CI, scripts and docs, and proposes checks, rubrics and facts for you to approve.")
	fmt.Printf("  For an agent that doesn't read %s, first run raisq skill <agent>, such as raisq skill claude-code.\n", skill.Standard)
	fmt.Println("  Commit .raisq/ and .agents/: they're the project's setup. .raisq/local/ is yours, and git ignores it.")
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

func write(p, body string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(body), 0o644)
}
