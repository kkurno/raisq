package skill

import (
	"os"
	"path/filepath"
	"strings"
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

// A formatter rewrapping the skill or changing its list and emphasis markers,
// as a pre-commit hook does, doesn't make it out of date; changed words do.
func TestFormattingIsntStale(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, []string{Standard}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, Dir(Standard), "SKILL.md")
	data, _ := os.ReadFile(p)
	formatted := strings.ReplaceAll(strings.ReplaceAll(string(data), "**", "__"), "\n- ", "\n* ")
	formatted = strings.ReplaceAll(formatted, ". ", ".\n")
	os.WriteFile(p, []byte(formatted), 0o644)
	if s := Stale(dir, []string{Standard}); len(s) != 0 {
		t.Fatalf("a formatted copy says the same, so it isn't stale: %v", s)
	}
	os.WriteFile(p, []byte(strings.Replace(string(data), "Never approve, answer or add a fact on your own", "Approve, answer or add facts as you see fit", 1)), 0o644)
	if s := Stale(dir, []string{Standard}); len(s) != 1 {
		t.Fatal("changed words make the skill stale")
	}
}
