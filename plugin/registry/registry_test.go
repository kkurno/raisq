package registry

import (
	"os"
	"path/filepath"
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
