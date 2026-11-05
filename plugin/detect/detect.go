// Package detect reads a project before .raisq/plugins/ exists and proposes
// the preset, the one-of choices and a check file per command it finds.
package detect

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtins/*.yaml
var builtins embed.FS

type Proposal struct {
	Presets   []string
	Builder   string
	Critic    string
	Isolation string
	Found     []string
	// Files is plugin files to write, by file name.
	Files   map[string]string
	Rubrics map[string]string
	Notes   []string
}

type pkgJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Main            string            `json:"main"`
	Types           string            `json:"types"`
	Exports         any               `json:"exports"`
}

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

func Detect(dir string) Proposal {
	p := Proposal{Files: map[string]string{}, Rubrics: map[string]string{}}

	var pkg pkgJSON
	hasPkg := false
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && json.Unmarshal(data, &pkg) == nil {
		hasPkg = true
	}
	deps := map[string]bool{}
	for k := range pkg.Dependencies {
		deps[k] = true
	}
	for k := range pkg.DevDependencies {
		deps[k] = true
	}

	switch {
	case deps["next"] || deps["vite"] || deps["astro"] || deps["@sveltejs/kit"] || deps["nuxt"] || deps["react-scripts"]:
		p.Presets = append(p.Presets, "web")
		p.Found = append(p.Found, "web app")
	}
	if hasPkg && (pkg.Main != "" || pkg.Types != "" || pkg.Exports != nil) && !contains(p.Presets, "web") {
		p.Presets = append(p.Presets, "library-ts")
	}
	if exists(dir, "go.mod") {
		p.Presets = append(p.Presets, "library-go")
		p.Found = append(p.Found, "Go module")
	}
	if exists(dir, "mkdocs.yml") || exists(dir, "docusaurus.config.js") || exists(dir, "docusaurus.config.ts") {
		p.Presets = append(p.Presets, "docs")
	}

	if hasPkg {
		pm := "npm"
		switch {
		case exists(dir, "pnpm-lock.yaml"):
			pm = "pnpm"
		case exists(dir, "yarn.lock"):
			pm = "yarn"
		}
		p.Found = append(p.Found, pm)
		run := func(script string) string {
			if pm == "npm" {
				return "npm run " + script
			}
			return pm + " " + script
		}
		prio := map[string]int{"build": 10, "lint": 15, "typecheck": 15, "test": 20}
		names := make([]string, 0, len(pkg.Scripts))
		for k := range pkg.Scripts {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, s := range names {
			pr, ok := prio[s]
			if !ok {
				continue
			}
			body := fmt.Sprintf("name: %s\nslot: checks\npriority: %d\nrun: %s\n", s, pr, run(s))
			if s == "test" {
				p.Found = append(p.Found, testRunner(deps))
				body += testExtras(deps, pkg.Scripts[s])
			}
			p.Files[s+".yaml"] = body
		}
		if start, ok := pkg.Scripts["start"]; ok && contains(p.Presets, "web") {
			_ = start
			cmd := run("start")
			if _, ok := pkg.Scripts["build"]; ok {
				cmd = run("build") + " && " + cmd
			}
			p.Files["site.yaml"] = fmt.Sprintf("name: site\nslot: provides\nrun: %s\nport: 3000\nready: http://localhost:3000\n", cmd)
		}
		if deps["@playwright/test"] {
			body, _ := Builtin("playwright")
			p.Files["playwright.yaml"] = body
			p.Found = append(p.Found, "Playwright")
		}
		if contains(p.Presets, "web") {
			body, _ := Builtin("lighthouse")
			if deps["lighthouse"] {
				p.Files["lighthouse.yaml"] = body
			}
			rub, _ := builtins.ReadFile("builtins/web-rubrics.yaml")
			p.Rubrics["web"] = string(rub)
			if _, ok := p.Files["site.yaml"]; ok {
				for _, b := range []string{"page-text", "contacts", "links"} {
					body, _ := Builtin(b)
					p.Files[b+".yaml"] = body
				}
			}
		}
	}
	if exists(dir, "go.mod") {
		p.Files["go-test.yaml"] = "name: go-test\nslot: checks\npriority: 20\nrun: go test ./...\nrun_one: go test ./$(dirname {file})\ntests: [\"**/*_test.go\"]\n"
		p.Files["go-vet.yaml"] = "name: go-vet\nslot: checks\npriority: 15\nrun: go vet ./...\n"
		body, _ := Builtin("go-api")
		p.Files["go-api.yaml"] = body
	}
	if exists(dir, "Makefile") && len(p.Files) == 0 {
		if data, _ := os.ReadFile(filepath.Join(dir, "Makefile")); strings.Contains(string(data), "\ntest:") {
			p.Files["make-test.yaml"] = "name: make-test\nslot: checks\npriority: 20\nrun: make test\n"
		}
	}
	if env := filepath.Join(dir, ".env.test"); exists(dir, ".env.test") && gitIgnored(dir, env) {
		p.Files["env-test.yaml"] = "name: env.test\nslot: provides\nfrom: host:.env.test\n"
		p.Notes = append(p.Notes, ".env.test goes to the checks only, after the builder has exited")
	}

	// One-of slots. Prefer a critic from another vendor.
	agents := []string{}
	for _, a := range []string{"claude-code", "cursor"} {
		bin := map[string]string{"claude-code": "claude", "cursor": "cursor-agent"}[a]
		if onPath(bin) {
			agents = append(agents, a)
		}
	}
	switch len(agents) {
	case 0:
		p.Notes = append(p.Notes, "no agent CLI found: RaisQ runs in manual mode until one is installed, and the critic needs one too")
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
		p.Notes = append(p.Notes, "no container tool found: only manual mode works until Docker or Podman is installed")
	}
	if !testsFound(p) {
		p.Notes = append(p.Notes, "no test command found: add a plugin file with slot: checks")
	}
	return p
}

func testsFound(p Proposal) bool {
	for name := range p.Files {
		if strings.Contains(p.Files[name], "slot: checks") {
			return true
		}
	}
	return false
}

func testRunner(deps map[string]bool) string {
	for _, r := range []string{"vitest", "jest", "mocha"} {
		if deps[r] {
			return r
		}
	}
	return "test script"
}

func testExtras(deps map[string]bool, script string) string {
	switch {
	case deps["vitest"] || strings.Contains(script, "vitest"):
		return "run_one: npx vitest run {file}\nlocks: [\"vitest.config.*\", \"vite.config.*\"]\ntests: [\"**/*.test.*\", \"**/*.spec.*\", \"**/__tests__/**\"]\n"
	case deps["jest"] || strings.Contains(script, "jest"):
		return "run_one: npx jest {file}\nlocks: [\"jest.config.*\"]\ntests: [\"**/*.test.*\", \"**/*.spec.*\", \"**/__tests__/**\"]\n"
	}
	return "tests: [\"**/*.test.*\", \"**/*.spec.*\"]\n"
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func gitIgnored(dir, path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = dir
	return cmd.Run() == nil
}
