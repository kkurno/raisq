// Package skill writes RaisQ's agent skill, in the Agent Skills format
// (agentskills.io), so people can use RaisQ from their agent's chat.
package skill

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// files is the skill folder: SKILL.md, which agents always load, and
// references/, which they open only when a task needs it.
//
//go:embed SKILL.md references
var files embed.FS

// Standard is the folder the Agent Skills standard names for a project's skills.
const Standard = ".agents/skills"

// Dir is where RaisQ's skill goes under a skills folder.
func Dir(skills string) string { return filepath.Join(skills, "raisq") }

// each calls fn with every file of the skill, by its path in the skill folder.
func each(fn func(rel string, data []byte) error) error {
	return fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return fn(filepath.FromSlash(p), data)
	})
}

// Write puts the skill under each skills folder, relative to the project, and
// returns the files it wrote.
func Write(project string, skills []string) ([]string, error) {
	var wrote []string
	for _, s := range skills {
		err := each(func(rel string, data []byte) error {
			p := filepath.Join(Dir(s), rel)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(project, p)), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(project, p), data, 0o644); err != nil {
				return err
			}
			wrote = append(wrote, p)
			return nil
		})
		if err != nil {
			return wrote, err
		}
	}
	return wrote, nil
}

// Written is the skills folders under project that already hold the skill.
func Written(project string, skills []string) []string {
	var out []string
	for _, s := range skills {
		if _, err := os.Stat(filepath.Join(project, Dir(s), "SKILL.md")); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// Stale is the skill folders under project where any file differs from this
// version's skill, or is missing.
func Stale(project string, skills []string) []string {
	var out []string
	for _, s := range Written(project, skills) {
		stale := false
		_ = each(func(rel string, data []byte) error {
			got, err := os.ReadFile(filepath.Join(project, Dir(s), rel))
			if err != nil || !bytes.Equal(got, data) {
				stale = true
			}
			return nil
		})
		if stale {
			out = append(out, Dir(s))
		}
	}
	return out
}
