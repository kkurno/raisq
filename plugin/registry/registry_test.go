package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every folder raisq skill can write to is kept from the builder, including
// a known agent's whose plugin file the project doesn't have.
func TestSkillFoldersArePrivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".raisq", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, "builder: cursor\ncritic: cursor-critic\n")
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{".raisq": false, ".agents/skills/raisq": false, ".claude/skills/raisq": false}
	for _, path := range p.Git.Private {
		if _, ok := want[path]; ok {
			want[path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("%s must be kept from the builder; private is %v", path, p.Git.Private)
		}
	}
	if f, ok := p.Agent("claude-code"); !ok || f.Skills != ".claude/skills" {
		t.Fatalf("a built-in agent is known without its file in the project: %+v", f)
	}
}

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	p := ConfigPath(filepath.Join(dir, ".raisq"))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newProject(t *testing.T, yaml string, plugins map[string]string) *Project {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".raisq", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, yaml)
	for name, body := range plugins {
		if err := os.WriteFile(filepath.Join(dir, ".raisq", "plugins", name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reopen(t *testing.T, p *Project) *Project {
	t.Helper()
	q, err := Open(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// Choosing a built-in agent brings its file in; a project's own file wins.
func TestSetBuilderBringsInTheBuiltInFile(t *testing.T) {
	p := newProject(t, "# mine\nbuilder: claude-code # who builds\ncritic: cursor-critic\n", map[string]string{
		"cursor-critic": "name: cursor-critic\nslot: model\nrun: x\n",
	})
	if _, err := p.Set("builder", "cursor", false); err != nil {
		t.Fatal(err)
	}
	q := reopen(t, p)
	if q.Config.Builder != "cursor" {
		t.Fatalf("builder %q", q.Config.Builder)
	}
	if _, ok := q.find("builder", "cursor"); !ok {
		t.Fatal("cursor's plugin file should be in the project")
	}
	data, _ := os.ReadFile(ConfigPath(p.Raisq))
	if !strings.Contains(string(data), "# mine") || !strings.Contains(string(data), "# who builds") {
		t.Fatalf("a person's comments must survive: %s", data)
	}

	mine := "name: claude-code\nslot: builder\nrun: my own command\n"
	os.WriteFile(filepath.Join(p.Raisq, "plugins", "claude-code.yaml"), []byte(mine), 0o644)
	q = reopen(t, q)
	if _, err := q.Set("builder", "claude-code", false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(p.Raisq, "plugins", "claude-code.yaml")); string(data) != mine {
		t.Fatal("the project's own plugin file must never be overwritten")
	}
}

func TestSetRefusesWhatWouldBreakARun(t *testing.T) {
	p := newProject(t, "builder: claude-code\ncritic: claude-code-critic\n", map[string]string{
		"claude-code":        "name: claude-code\nslot: builder\nrun: x\n",
		"claude-code-critic": "name: claude-code-critic\nslot: model\nrun: x\n",
	})
	before, _ := os.ReadFile(ConfigPath(p.Raisq))
	for _, c := range [][2]string{
		{"critic", "claude-code"}, // a builder can't be the critic
		{"builder", "nope"},
		{"attempts", "0"},
		{"batch", "8"},
		{"keep_artifacts", "30"},
		{"spot_check", "2"},
		{"isolation", "vm"},
		{"colour", "red"},
	} {
		if _, err := p.Set(c[0], c[1], false); err == nil {
			t.Errorf("%s %s should be refused", c[0], c[1])
		}
	}
	if _, err := p.Set("isolation", "none", false); err == nil {
		t.Error("isolation none needs a person's yes")
	}
	if after, _ := os.ReadFile(ConfigPath(p.Raisq)); string(after) != string(before) {
		t.Fatalf("a refused change must leave raisq.yaml as it was:\n%s", after)
	}
	if _, err := p.Unset("builder"); err == nil {
		t.Error("the builder has no default to go back to")
	}
}

// 0 is a real choice for spot_check, not "unset".
func TestSpotCheckZeroMeansNone(t *testing.T) {
	p := newProject(t, "builder: x\ncritic: y\n", nil)
	if p.SpotShare() != 0.2 {
		t.Fatal("default share")
	}
	if _, err := p.Set("spot_check", "0", false); err != nil {
		t.Fatal(err)
	}
	if reopen(t, p).SpotShare() != 0 {
		t.Fatal("spot_check 0 must turn spot checks off")
	}
}

func TestRemovePluginKeepsWhatARunNeeds(t *testing.T) {
	p := newProject(t, "builder: claude-code\ncritic: cursor-critic\n", map[string]string{
		"claude-code":   "name: claude-code\nslot: builder\nrun: x\n",
		"cursor-critic": "name: cursor-critic\nslot: model\nrun: x\n",
		"site":          "name: site\nslot: provides\nrun: npm start\n",
		"links":         "name: links\nslot: checks\nrun: builtin:links\nneeds: [site]\n",
	})
	for _, n := range []string{"claude-code", "cursor-critic", "site", "nope"} {
		if _, _, err := p.RemovePlugin(n); err == nil {
			t.Errorf("removing %s should be refused", n)
		}
	}
	if _, _, err := p.RemovePlugin("links"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.RemovePlugin("site"); err != nil {
		t.Fatal("once nothing needs it, a provider can go:", err)
	}
}

// A project set up before local/ moves this machine's state there on first
// open, keeping a person's own .gitignore lines and losing nothing.
func TestOldLayoutMovesToLocal(t *testing.T) {
	dir := t.TempDir()
	raisq := filepath.Join(dir, ".raisq")
	for p, body := range map[string]string{
		"raisq.yaml":                 "builder: claude-code # mine\ncritic: cursor-critic\n",
		"tasks/t1/state.yaml":        "state: ready\nadded: x\n",
		"tasks/t1/spec.yaml":         "name: t1\ntitle: T\ncriteria: [{id: c1, text: x}]\n",
		"history/facts/abc/b.yaml":   "[]\n",
		"baselines/c.json":           "{}",
		"batch-report.md":            "# report\n",
		".gitignore":                 "tasks/*/runs/\ntasks/*/history/\nhistory/\nbaselines/\nmy-own-line\n",
		"facts/business.yaml":        "- id: business/name\n  text: Shop\n  tag: confirmed\n",
		"plugins/cursor-critic.yaml": "name: cursor-critic\nslot: model\nrun: x\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(raisq, p)), 0o755)
		os.WriteFile(filepath.Join(raisq, p), []byte(body), 0o644)
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Config.Builder != "claude-code" || len(p.Moved) == 0 {
		t.Fatalf("settings should load from their new place: %+v moved %v", p.Config, p.Moved)
	}
	if st, err := p.Store.LoadState("t1"); err != nil || st.State != "ready" {
		t.Fatalf("tasks must survive the move: %v %+v", err, st)
	}
	for _, gone := range []string{"raisq.yaml", "tasks", "history", "baselines", "batch-report.md"} {
		if _, err := os.Stat(filepath.Join(raisq, gone)); err == nil {
			t.Errorf("%s should have moved into local/", gone)
		}
	}
	for _, kept := range []string{"facts/business.yaml", "plugins/cursor-critic.yaml", "local/snapshots/facts/abc/b.yaml", "local/baselines/c.json", "local/batch-report.md"} {
		if _, err := os.Stat(filepath.Join(raisq, kept)); err != nil {
			t.Errorf("%s should exist after the move", kept)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(raisq, ".gitignore")); string(data) != "my-own-line\n" {
		t.Fatalf("only RaisQ's own ignore lines go: %q", data)
	}
	if data, _ := os.ReadFile(ConfigPath(raisq)); !strings.Contains(string(data), "# mine") {
		t.Fatal("the config moves as it was, comments included")
	}
	if q := reopen(t, p); len(q.Moved) != 0 {
		t.Fatal("the move happens once")
	}
}

// A teammate who clones a project has its shared files but no settings for
// their own machine: every command says to run raisq init.
func TestClonedProjectNeedsThisMachineSetUp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".raisq", "facts"), 0o755)
	if _, err := Open(dir); err != ErrNoMachine {
		t.Fatalf("want ErrNoMachine, got %v", err)
	}
}
