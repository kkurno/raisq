package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOneAgentUsesSameVendor(t *testing.T) {
	onPath = func(bin string) bool { return bin == "claude" }
	p := DetectMachine()
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
