package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Contract checks a plugin file against its slot's contract: the fields the
// slot needs, no template left unfilled, and a model's answer to a fixed
// question. It never runs a check's command: that's the project's code, which
// runs only in isolation, through the trial run. It returns what's wrong.
func Contract(ctx context.Context, body, dir string) []string {
	var f File
	if err := yaml.Unmarshal([]byte(body), &f); err != nil {
		return []string{"not valid YAML: " + err.Error()}
	}
	var bad []string
	if f.Name == "" {
		bad = append(bad, "needs a name")
	}
	if strings.Contains(body, "<") && regexp.MustCompile(`<[a-z][^>]*>`).MatchString(stripComments(body)) {
		bad = append(bad, "has template parts left to fill: "+strings.Join(regexp.MustCompile(`<[a-z][^>]*>`).FindAllString(stripComments(body), 5), ", "))
	}
	need := map[string][]string{
		"builder":     {"run", "vendor"},
		"model":       {"run", "vendor"},
		"checks":      {"run"},
		"extractor":   {"run"},
		"spec-source": {"run", "matches"},
		"report":      {"run"},
	}
	fields, ok := need[f.Slot]
	if f.Slot == "provides" {
		ok = true
		if f.Run == "" && f.From == "" && f.Image == "" {
			bad = append(bad, "a provider needs run, from or image")
		}
		if f.Image != "" && f.For == "builder" {
			bad = append(bad, "a service (image:) is for the checks, never the builder")
		}
		if f.For == "builder" && (f.From == "" || !strings.HasSuffix(f.From, ":ro")) {
			bad = append(bad, "a builder provider is a read-only mount: from: host:<path>:ro")
		}
	}
	if !ok {
		return append(bad, fmt.Sprintf("unknown slot %q", f.Slot))
	}
	have := map[string]string{"run": f.Run, "vendor": f.Vendor, "matches": f.Matches}
	for _, k := range fields {
		if strings.TrimSpace(have[k]) == "" {
			bad = append(bad, "the "+f.Slot+" slot needs "+k)
		}
	}
	if f.Dir != "" && (strings.HasPrefix(f.Dir, "/") || strings.Contains("/"+f.Dir+"/", "/../")) {
		bad = append(bad, "dir is a folder inside the project, such as apps/api")
	}
	if f.Slot == "builder" && !strings.Contains(f.Run, "{prompt}") {
		bad = append(bad, "a builder's run must read {prompt}")
	}
	if f.Slot == "spec-source" && f.Matches != "" {
		if _, err := regexp.Compile(f.Matches); err != nil {
			bad = append(bad, "matches isn't a regular expression: "+err.Error())
		}
	}
	if f.Tool != nil && f.Tool.Find != "" {
		bin := strings.Fields(f.Tool.Find)[0]
		if _, err := exec.LookPath(bin); err != nil && f.Slot != "builder" && f.Slot != "checks" {
			bad = append(bad, bin+" isn't installed here")
		}
	}
	if len(bad) > 0 {
		return bad
	}
	if f.Slot == "checks" {
		if svc := startsService(f.Run); svc != "" {
			bad = append(bad, "its run starts a service with "+svc+": a check runs one command of the project's. Put the service in a provider with image:, and name it in needs:")
		}
	}
	if len(bad) > 0 {
		return bad
	}
	if f.Slot == "checks" && f.Dir != "" {
		if info, err := os.Stat(filepath.Join(dir, f.Dir)); err != nil || !info.IsDir() {
			return []string{"dir " + f.Dir + " isn't a folder in the project"}
		}
	}
	// A model runs on the host by design, with no tools, so asking it a fixed
	// question is safe and shows it answers.
	if f.Slot == "model" {
		out, err := Model{F: f, Timeout: 3 * time.Minute}.Ask(ctx, `Answer with JSON only: {"ok": true}`)
		if err != nil {
			return []string{"the example failed: " + err.Error()}
		}
		if !strings.Contains(out, "ok") {
			return []string{"the example's answer wasn't JSON: " + strings.TrimSpace(out)}
		}
	}
	return nil
}

func stripComments(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// containerTools start containers. A check that runs one manages its own
// services, which ties it to one machine's setup and runs outside isolation's
// control; providers are how a check gets a service.
var containerTools = regexp.MustCompile(`(^|[\s;&|(])(docker|podman|docker-compose|nerdctl)\s+(run|compose|start|create|up)\b|(^|[\s;&|(])docker-compose\s+up\b`)

// startsService is the container command a check's run starts, or "".
func startsService(run string) string {
	if m := containerTools.FindString(run); m != "" {
		return strings.TrimLeft(m, " \t;&|(")
	}
	return ""
}
