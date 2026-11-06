package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// The setup guide goes with the skill, and an old copy of either file is
// found and refreshed.
func TestWriteAndStaleCoverEveryFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, []string{Standard}); err != nil {
		t.Fatal(err)
	}
	guide := filepath.Join(dir, Dir(Standard), "references", "setup.md")
	if _, err := os.Stat(guide); err != nil {
		t.Fatal("the setup guide must be written with the skill")
	}
	if s := Stale(dir, []string{Standard}); len(s) != 0 {
		t.Fatalf("a fresh copy isn't stale: %v", s)
	}
	os.WriteFile(guide, []byte("old"), 0o644)
	if s := Stale(dir, []string{Standard}); len(s) != 1 {
		t.Fatal("an old setup guide makes the skill stale")
	}
	os.Remove(guide)
	if s := Stale(dir, []string{Standard}); len(s) != 1 {
		t.Fatal("a missing setup guide makes the skill stale")
	}
}
