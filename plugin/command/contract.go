package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/plugin/proc"
	"github.com/kkurno/raisq/slot"
)

// Contract checks a plugin file against its slot's contract: the fields
// the slot needs, no template left unfilled, and, where it's safe to, a run
// against the slot's example. It returns what's wrong.
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
		if f.Run == "" && f.From == "" {
			bad = append(bad, "a provider needs run or from")
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
	// Run the slot's example where it can't touch anything: a model answers a
	// fixed question; a check runs in the project but changes nothing.
	switch f.Slot {
	case "model":
		out, err := Model{F: f, Timeout: 3 * time.Minute}.Ask(ctx, `Answer with JSON only: {"ok": true}`)
		if err != nil {
			return []string{"the example failed: " + err.Error()}
		}
		if !strings.Contains(out, "ok") {
			return []string{"the example's answer wasn't JSON: " + strings.TrimSpace(out)}
		}
	case "checks":
		s, err := hostSession(dir)
		if err != nil {
			return []string{err.Error()}
		}
		if r := (Check{F: f}).Run(ctx, s, nil); r.Status == "blocked" {
			return []string{"the example run couldn't find its tool: " + strings.TrimSpace(r.Output)}
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

type hostSess struct{ dir string }

func hostSession(dir string) (slot.Session, error) { return hostSess{dir}, nil }

func (h hostSess) Paths() slot.Paths { return slot.Paths{Workdir: h.dir} }
func (h hostSess) Exec(ctx context.Context, cmd string, env map[string]string) (slot.ExecResult, error) {
	c := proc.Shell(ctx, cmd)
	c.Dir = h.dir
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return slot.ExecResult{Exit: ee.ExitCode(), Output: string(out)}, nil
	}
	return slot.ExecResult{Output: string(out)}, err
}
func (h hostSess) Background(context.Context, string, map[string]string) error { return nil }
func (h hostSess) URL(int) (string, error)                                     { return "", nil }
func (h hostSess) Stop(context.Context) error                                  { return nil }
