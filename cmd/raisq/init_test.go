package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// gitProject makes an empty committed git repo and makes it the current
// folder for the test.
func gitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, c := range []string{"init -q -b main", "config user.email t@t", "config user.name t", "commit -q --allow-empty -m init"} {
		cmd := exec.Command("git", strings.Fields(c)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s", c, out)
		}
	}
	// The CLI works in the current folder; go back after the test.
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	return dir
}

func initYes(t *testing.T) {
	t.Helper()
	if err := cmdInit(context.Background(), []string{"--yes"}); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// raisq init is safe to run again: it fills in what's missing, such as
// emptied plugin and skill folders or an old skill, and never replaces what
// a person changed or a teammate committed.
func TestInitFillsInWhatsMissing(t *testing.T) {
	dir := gitProject(t)
	initYes(t)
	cfg := filepath.Join(dir, ".raisq", "local", "config.yaml")
	guide := filepath.Join(dir, ".agents", "skills", "raisq", "references", "setup.md")
	for _, p := range []string{cfg, guide} {
		if !exists(p) {
			t.Fatalf("a first init writes %s", p)
		}
	}
	initYes(t) // nothing missing: no error

	// The state a person can end up in: settings kept, the rest emptied.
	plugins := filepath.Join(dir, ".raisq", "plugins")
	entries, _ := os.ReadDir(plugins)
	if len(entries) == 0 {
		t.Fatal("init writes the chosen agents' plugin files")
	}
	for _, e := range entries {
		os.Remove(filepath.Join(plugins, e.Name()))
	}
	os.RemoveAll(filepath.Join(dir, ".agents", "skills", "raisq", "references"))
	os.WriteFile(filepath.Join(dir, ".agents", "skills", "raisq", "SKILL.md"), []byte("old"), 0o644)
	settings, _ := os.ReadFile(cfg)
	initYes(t)
	if after, _ := os.ReadFile(cfg); string(after) != string(settings) {
		t.Fatal("init keeps this machine's settings")
	}
	if entries, _ := os.ReadDir(plugins); len(entries) == 0 {
		t.Fatal("init puts back the agents' plugin files")
	}
	if !exists(guide) {
		t.Fatal("init puts back the skill's setup guide")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".agents", "skills", "raisq", "SKILL.md")); string(data) == "old" {
		t.Fatal("init refreshes an old skill")
	}

	// A plugin file a person edited is theirs.
	entries, _ = os.ReadDir(plugins)
	mine := filepath.Join(plugins, entries[0].Name())
	edited := "name: " + strings.TrimSuffix(entries[0].Name(), ".yaml") + "\nslot: builder\nvendor: v\nrun: mine {prompt}\n"
	os.WriteFile(mine, []byte(edited), 0o644)
	initYes(t)
	if data, _ := os.ReadFile(mine); string(data) != edited {
		t.Fatal("init never replaces a plugin file that's there")
	}
}

// An emptied agent skill folder, such as Claude Code's, is filled in again.
func TestInitRefillsAnAgentsSkillFolder(t *testing.T) {
	dir := gitProject(t)
	initYes(t)
	claude := filepath.Join(dir, ".claude", "skills", "raisq")
	os.MkdirAll(claude, 0o755)
	initYes(t)
	if !exists(filepath.Join(claude, "SKILL.md")) {
		t.Fatal("init fills in an agent's skill folder that's there but empty")
	}
}

// A teammate who clones the project gets settings for their own machine;
// with no terminal and no --yes, nothing is written and it says how.
func TestInitOnAClone(t *testing.T) {
	dir := gitProject(t)
	initYes(t)
	cfg := filepath.Join(dir, ".raisq", "local", "config.yaml")
	os.Remove(cfg)
	if err := cmdInit(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("without a terminal, init asks for --yes: %v", err)
	}
	if exists(cfg) {
		t.Fatal("nothing is written without a yes")
	}
	initYes(t)
	if !exists(cfg) {
		t.Fatal("init sets up this machine")
	}
}

// With isolation: none the builder is the agent on this machine, using its
// own login, so doctor doesn't ask for the login RaisQ would copy into a
// container; it still says when the agent isn't installed.
func TestDoctorWithIsolationNone(t *testing.T) {
	dir := gitProject(t)
	initYes(t)
	plugins := filepath.Join(dir, ".raisq", "plugins")
	os.WriteFile(filepath.Join(plugins, "agent.yaml"), []byte("name: agent\nslot: builder\nvendor: v\ntool:\n  find: true --version\nrun: x {prompt}\nlogin:\n  - file: ~/.no-such-login.json\n    to: /home/raisq/.l\n"), 0o644)
	cfg := filepath.Join(dir, ".raisq", "local", "config.yaml")
	data, _ := os.ReadFile(cfg)
	set := func(isolation string) {
		out := strings.Replace(string(data), "builder: claude-code", "builder: agent", 1)
		out = regexp.MustCompile(`(?m)^isolation: .*$`).ReplaceAllString(out, "isolation: "+isolation)
		os.WriteFile(cfg, []byte(out), 0o644)
	}
	set("none")
	proj, err := open()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range doctor(context.Background(), proj) {
		if strings.Contains(p, "not logged in") {
			t.Fatalf("with isolation none, doctor mustn't ask for a login it never copies: %s", p)
		}
	}
	os.WriteFile(filepath.Join(plugins, "agent.yaml"), []byte("name: agent\nslot: builder\nvendor: v\ntool:\n  find: no-such-agent-xyz --version\nrun: x {prompt}\n"), 0o644)
	proj, _ = open()
	if !strings.Contains(strings.Join(doctor(context.Background(), proj), "\n"), "no-such-agent-xyz") {
		t.Fatal("with isolation none, doctor says when the agent isn't installed")
	}
}
