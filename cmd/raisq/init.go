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

	"raisq/domain"
	"raisq/plugin/detect"
	"raisq/plugin/registry"
)

func cmdInit(ctx context.Context, args []string) error {
	fs := flags("init")
	yes := fs.Bool("yes", false, "accept what init proposes without asking")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	dir := wd()
	raisq := filepath.Join(dir, ".raisq")
	if _, err := os.Stat(filepath.Join(raisq, "raisq.yaml")); err == nil {
		return fmt.Errorf("already set up: .raisq/raisq.yaml exists")
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
		ok, err := newTTY().Confirm("Write these into .raisq/?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("nothing written")
		}
	}

	cfg := registry.Config{Builder: p.Builder, Critic: p.Critic, Isolation: p.Isolation}
	switch len(p.Presets) {
	case 0:
	case 1:
		cfg.Preset = p.Presets[0]
	default:
		cfg.Preset = p.Presets
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(raisq, "raisq.yaml"), "# .raisq/raisq.yaml · written by raisq init\n"+string(data)); err != nil {
		return err
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
	for _, d := range []string{"facts", "tasks"} {
		if err := os.MkdirAll(filepath.Join(raisq, d), 0o755); err != nil {
			return err
		}
	}
	// Run history, pinned revisions and baselines are local working state.
	if err := write(filepath.Join(raisq, ".gitignore"), "tasks/*/runs/\ntasks/*/history/\nhistory/\nbaselines/\n"); err != nil {
		return err
	}

	if contains(p.Presets, "web") || contains(p.Presets, "docs") {
		if err := seedFacts(dir, *yes); err != nil {
			return err
		}
	}
	fmt.Println("  Ready. Next: raisq task \"what you want built\"")
	return nil
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
			f = domain.Fact{ID: q.id, Text: a, Kind: q.kind, Tag: domain.Confirmed, SaidBy: "implementor", SaidOn: today}
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
