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
