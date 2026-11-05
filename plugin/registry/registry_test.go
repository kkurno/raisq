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
	if err := os.WriteFile(filepath.Join(dir, ".raisq", "raisq.yaml"), []byte("builder: cursor\ncritic: cursor-critic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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

func newProject(t *testing.T, yaml string, plugins map[string]string) *Project {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".raisq", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".raisq", "raisq.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
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
	data, _ := os.ReadFile(filepath.Join(p.Raisq, "raisq.yaml"))
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
	before, _ := os.ReadFile(filepath.Join(p.Raisq, "raisq.yaml"))
	for _, c := range [][2]string{
		{"critic", "claude-code"}, // a builder can't be the critic
		{"builder", "nope"},
		{"attempts", "0"},
		{"batch", "8"},
		{"keep_artifacts", "30"},
		{"spot_check", "2"},
		{"isolation", "vm"},
		{"preset", "web"},
		{"colour", "red"},
	} {
		if _, err := p.Set(c[0], c[1], false); err == nil {
			t.Errorf("%s %s should be refused", c[0], c[1])
		}
	}
	if _, err := p.Set("isolation", "none", false); err == nil {
		t.Error("isolation none needs a person's yes")
	}
	if after, _ := os.ReadFile(filepath.Join(p.Raisq, "raisq.yaml")); string(after) != string(before) {
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
