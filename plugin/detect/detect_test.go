package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectNextApp(t *testing.T) {
	onPath = func(bin string) bool { return bin == "docker" || bin == "claude" || bin == "cursor-agent" }
	defer func() { onPath = func(bin string) bool { return false } }()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
	  "scripts": {"build":"next build","start":"next start","test":"vitest run","lint":"next lint","dev":"next"},
	  "dependencies": {"next":"15"}, "devDependencies": {"vitest":"2","@playwright/test":"1"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644)
	p := Detect(dir)
	if len(p.Presets) != 1 || p.Presets[0] != "web" {
		t.Fatalf("presets %v", p.Presets)
	}
	if p.Builder != "claude-code" || p.Critic != "cursor-critic" {
		t.Fatalf("two vendors should split builder and critic: %s %s", p.Builder, p.Critic)
	}
	for _, f := range []string{"build.yaml", "test.yaml", "lint.yaml", "site.yaml", "playwright.yaml"} {
		if _, ok := p.Files[f]; !ok {
			t.Errorf("missing %s", f)
		}
	}
	if _, ok := p.Files["dev.yaml"]; ok {
		t.Error("dev is not a check")
	}
	if !strings.Contains(p.Files["test.yaml"], "run_one: npx vitest run {file}") {
		t.Error("vitest should know how to run one file")
	}
	if !strings.Contains(p.Files["site.yaml"], "npm run build && npm run start") {
		t.Error(p.Files["site.yaml"])
	}
}

func TestOneAgentUsesSameVendor(t *testing.T) {
	onPath = func(bin string) bool { return bin == "claude" }
	p := Detect(t.TempDir())
	if p.Builder != "claude-code" || p.Critic != "claude-code-critic" {
		t.Fatal(p.Builder, p.Critic)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "same vendor as the builder") {
		t.Error("should say so")
	}
}

func TestNodeMajor(t *testing.T) {
	for _, c := range []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{".nvmrc": "24\n"}, "24"},
		{map[string]string{".nvmrc": "v20.11.1"}, "20"},
		{map[string]string{".nvmrc": "lts/iron"}, "20"},
		{map[string]string{".node-version": "22.3.0"}, "22"},
		{map[string]string{".tool-versions": "golang 1.23\nnodejs 18.20.0\n"}, "18"},
		{map[string]string{"package.json": `{"engines":{"node":">=24.0.0"}}`}, "24"},
		{map[string]string{"package.json": `{"engines":{"node":"^20 || ^22"}}`}, "20"},
		{map[string]string{".nvmrc": "24", "package.json": `{"engines":{"node":">=18"}}`}, "24"},
		{map[string]string{"package.json": `{"name":"x"}`}, ""},
		{nil, ""},
	} {
		dir := t.TempDir()
		for n, body := range c.files {
			os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644)
		}
		if got := NodeMajor(dir); got != c.want {
			t.Errorf("%v: got %q, want %q", c.files, got, c.want)
		}
	}
}

// A build or a lint passes on code that does the wrong thing, so a project
// with only those is told it has no tests.
func TestOnlyABuildIsNoTests(t *testing.T) {
	onPath = func(string) bool { return false }
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"build":"turbo run build","lint":"turbo run lint"}}`), 0o644)
	p := Detect(dir)
	if !strings.Contains(strings.Join(p.Notes, "\n"), "no test command found") {
		t.Fatalf("a build and a lint aren't tests: %v", p.Notes)
	}
}
