package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFile is this machine's settings, under local/.
const ConfigFile = "config.yaml"

// EnsureLocal makes local/ and the .gitignore that keeps all of it out of git.
func EnsureLocal(raisq string) error {
	dir := filepath.Join(raisq, Local)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(ignore); err == nil {
		return nil
	}
	return os.WriteFile(ignore, []byte("# This machine's RaisQ state: tasks, runs and settings. Never committed.\n*\n"), 0o644)
}

// oldIgnore is what raisq init wrote to .raisq/.gitignore before local/.
var oldIgnore = map[string]bool{
	"tasks/*/runs/": true, "tasks/*/history/": true, "history/": true,
	"baselines/": true, "batch-report.md": true,
}

// Migrate moves a project from the layout before local/: raisq.yaml, tasks,
// fact history, baselines and the batch report move under local/, and the
// lines RaisQ wrote to .raisq/.gitignore go. It returns what it moved, and
// does nothing when there's nothing to move.
func Migrate(raisq string) ([]string, error) {
	moves := [][2]string{
		{"raisq.yaml", filepath.Join(Local, ConfigFile)},
		{"tasks", filepath.Join(Local, "tasks")},
		{"history", filepath.Join(Local, "snapshots")},
		{"baselines", filepath.Join(Local, "baselines")},
		{"batch-report.md", filepath.Join(Local, "batch-report.md")},
	}
	var todo [][2]string
	for _, m := range moves {
		if _, err := os.Stat(filepath.Join(raisq, m[0])); err == nil {
			todo = append(todo, m)
		}
	}
	if len(todo) == 0 {
		return nil, nil
	}
	if err := EnsureLocal(raisq); err != nil {
		return nil, err
	}
	var moved []string
	for _, m := range todo {
		from, to := filepath.Join(raisq, m[0]), filepath.Join(raisq, m[1])
		if _, err := os.Stat(to); err == nil {
			return moved, fmt.Errorf("both %s and %s exist: keep one, then run the command again", from, to)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return moved, err
		}
		if err := os.Rename(from, to); err != nil {
			return moved, err
		}
		moved = append(moved, m[0])
	}
	return moved, dropOldIgnore(filepath.Join(raisq, ".gitignore"))
}

// dropOldIgnore removes the lines RaisQ wrote, keeping any a person added.
func dropOldIgnore(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var kept []string
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if !oldIgnore[strings.TrimSpace(l)] {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 || len(kept) == 1 && strings.TrimSpace(kept[0]) == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}
