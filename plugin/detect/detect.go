// Package detect finds what this machine has: the agent CLIs and container
// tool installed, and the Node version a project asks for. It also holds
// RaisQ's built-in plugin files. Working out how a project is checked needs
// judgement, so the agent does it, through the CLI.
package detect

import (
	"embed"
	"os/exec"
	"strings"
)

//go:embed builtins/*.yaml
var builtins embed.FS

// Builtin is one of RaisQ's plugin files, by name.
func Builtin(name string) (string, bool) {
	data, err := builtins.ReadFile("builtins/" + name + ".yaml")
	return string(data), err == nil
}

// Builtins is every built-in plugin file, by name.
func Builtins() map[string]string {
	out := map[string]string{}
	entries, _ := builtins.ReadDir("builtins")
	for _, e := range entries {
		n := strings.TrimSuffix(e.Name(), ".yaml")
		out[n], _ = Builtin(n)
	}
	return out
}

// onPath is replaceable in tests.
var onPath = func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// Machine is what this machine has: the agents and container tool found on
// it, and the plugin files for the agents chosen.
type Machine struct {
	Builder, Critic, Isolation string
	Found, Notes               []string
	Files                      map[string]string
}

// DetectMachine picks the builder, critic and isolation from what's installed.
func DetectMachine() Machine {
	p := Machine{Files: map[string]string{}}
	// Prefer a critic from another vendor.
	agents := []string{}
	for _, a := range []string{"claude-code", "cursor"} {
		bin := map[string]string{"claude-code": "claude", "cursor": "cursor-agent"}[a]
		if onPath(bin) {
			agents = append(agents, a)
		}
	}
	switch len(agents) {
	case 0:
		p.Notes = append(p.Notes, "no agent CLI found: install one, then run raisq config builder and critic. Until then only manual mode works")
		p.Builder, p.Critic = "claude-code", "claude-code-critic"
	case 1:
		p.Builder, p.Critic = agents[0], agents[0]+"-critic"
		p.Notes = append(p.Notes, "one agent CLI: the critic is from the same vendor as the builder, and every review pack says so")
	default:
		p.Builder, p.Critic = agents[0], agents[1]+"-critic"
	}
	p.Found = append(p.Found, agents...)
	for _, n := range []string{p.Builder, p.Critic} {
		if body, ok := Builtin(n); ok {
			p.Files[n+".yaml"] = body
		}
	}

	switch {
	case onPath("docker"):
		p.Isolation = "docker"
		p.Found = append(p.Found, "Docker")
	case onPath("podman"):
		p.Isolation = "podman"
		p.Found = append(p.Found, "Podman")
	default:
		p.Isolation = "docker"
		p.Notes = append(p.Notes, "no container tool found: nothing can run until Docker or Podman is installed")
	}
	return p
}
