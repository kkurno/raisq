// Package skill writes RaisQ's agent skill, in the Agent Skills format
// (agentskills.io), so people can use RaisQ from their agent's chat.
package skill

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed SKILL.md
var body []byte

// Standard is the folder the Agent Skills standard names for a project's skills.
const Standard = ".agents/skills"

// Dir is where RaisQ's skill goes under a skills folder.
func Dir(skills string) string { return filepath.Join(skills, "raisq") }

// Write puts the skill under each skills folder, relative to the project, and
// returns the files it wrote.
func Write(project string, skills []string) ([]string, error) {
	var wrote []string
	for _, s := range skills {
		p := filepath.Join(Dir(s), "SKILL.md")
		if err := os.MkdirAll(filepath.Join(project, Dir(s)), 0o755); err != nil {
			return wrote, err
		}
		if err := os.WriteFile(filepath.Join(project, p), body, 0o644); err != nil {
			return wrote, err
		}
		wrote = append(wrote, p)
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

// Stale is the skills folders under project whose copy differs from this
// version's skill.
func Stale(project string, skills []string) []string {
	var out []string
	for _, s := range Written(project, skills) {
		data, err := os.ReadFile(filepath.Join(project, Dir(s), "SKILL.md"))
		if err != nil || !bytes.Equal(data, body) {
			out = append(out, Dir(s))
		}
	}
	return out
}
