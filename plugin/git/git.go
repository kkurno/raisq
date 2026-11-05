// Package git fills the checkout slot with git.
package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Git struct {
	Project string // the user's repo
	Work    string // where task checkouts are made, outside the repo
	// Private is RaisQ's own files in the repo: never in a checkout, and never
	// in a task's commits or diffs.
	Private []string
}

func New(project, work string, private ...string) *Git {
	return &Git{Project: project, Work: work, Private: append([]string{".raisq"}, private...)}
}

// outside is a pathspec for the whole repo except RaisQ's own files.
func (g *Git) outside() []string {
	spec := []string{"--", "."}
	for _, p := range g.Private {
		spec = append(spec, ":(exclude)"+p)
	}
	return spec
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// A checkout must never pick up the user's hooks, signing or credentials.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func (g *Git) Head(ctx context.Context) (string, error) {
	return run(ctx, g.Project, "rev-parse", "HEAD")
}

func (g *Git) Clean(ctx context.Context) (bool, error) {
	out, err := run(ctx, g.Project, append([]string{"status", "--porcelain"}, g.outside()...)...)
	return out == "", err
}

func (g *Git) BranchHead(ctx context.Context, branch string) (string, error) {
	return run(ctx, g.Project, "rev-parse", "--verify", "refs/heads/"+branch)
}

// Prepare clones the project into Work, never as a worktree, which would put
// the main repo's .git inside isolation. The clone has no remote. Each call
// gets a fresh path: a container tool can hold on to a deleted one.
func (g *Git) Prepare(ctx context.Context, task, branch, base string) (string, error) {
	if err := os.MkdirAll(g.Work, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(g.Work, task+"-")
	if err != nil {
		return "", err
	}
	if err := os.Remove(dir); err != nil {
		return "", err
	}
	if _, err := run(ctx, g.Work, "clone", "--quiet", "--no-hardlinks", "--no-checkout", g.Project, dir); err != nil {
		return "", err
	}
	start := base
	if _, err := g.BranchHead(ctx, branch); err == nil {
		start = "origin/" + branch
	}
	steps := [][]string{
		{"remote", "remove", "origin"},
		{"config", "user.name", "raisq"},
		{"config", "user.email", "raisq@localhost"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", "/dev/null"},
	}
	if _, err := run(ctx, dir, "checkout", "--quiet", "-B", branch, start); err != nil {
		return "", err
	}
	for _, s := range steps {
		if _, err := run(ctx, dir, s...); err != nil {
			return "", err
		}
	}
	// RaisQ's files are never inside isolation: the builder reads the frozen
	// spec through its own mount, and has no use for the rest.
	for _, p := range g.Private {
		if err := os.RemoveAll(filepath.Join(dir, p)); err != nil {
			return "", err
		}
	}
	return dir, g.snapshotGit(dir)
}

// snapshotGit records the hooks and config so the audit can see a change.
func (g *Git) snapshotGit(dir string) error {
	sum, err := gitSum(dir)
	if err != nil {
		return err
	}
	return os.WriteFile(dir+".gitsum", []byte(sum), 0o644)
}

func gitSum(dir string) (string, error) {
	h := sha256.New()
	gitDir := filepath.Join(dir, ".git")
	var names []string
	for _, sub := range []string{"config", "hooks"} {
		p := filepath.Join(gitDir, sub)
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				names = append(names, path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	sort.Strings(names)
	for _, n := range names {
		data, err := os.ReadFile(n)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(gitDir, n)
		fmt.Fprintf(h, "%s\x00", rel)
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (g *Git) AuditGit(ctx context.Context, dir string) error {
	want, err := os.ReadFile(dir + ".gitsum")
	if err != nil {
		return err
	}
	got, err := gitSum(dir)
	if err != nil {
		return err
	}
	if got != string(want) {
		return fmt.Errorf("the checkout's .git hooks or config changed")
	}
	return nil
}

// Commit stages everything except RaisQ's files and commits as raisq.
func (g *Git) Commit(ctx context.Context, dir, message string) (string, bool, error) {
	if _, err := run(ctx, dir, append([]string{"add", "-A"}, g.outside()...)...); err != nil {
		return "", false, err
	}
	if out, _ := run(ctx, dir, "diff", "--cached", "--name-only"); out == "" {
		head, err := run(ctx, dir, "rev-parse", "HEAD")
		return head, false, err
	}
	if _, err := run(ctx, dir, "commit", "--quiet", "--no-verify", "-m", message); err != nil {
		return "", false, err
	}
	head, err := run(ctx, dir, "rev-parse", "HEAD")
	return head, true, err
}

func (g *Git) Bring(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, g.Project, "fetch", "--quiet", "--no-tags", dir, "+refs/heads/"+branch+":refs/heads/"+branch)
	return err
}

func (g *Git) Diff(ctx context.Context, dir, base, head string) (string, error) {
	return run(ctx, dir, append([]string{"diff", "--no-color", "--no-ext-diff", base, head}, g.outside()...)...)
}

func (g *Git) Changed(ctx context.Context, dir, base, head string) ([]string, error) {
	out, err := run(ctx, dir, "diff", "--name-only", base, head)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Restore puts files matching globs back to base. Files the task added that
// match are returned rather than removed: they're new tests to prove.
func (g *Git) Restore(ctx context.Context, dir, base string, globs []string) ([]string, []string, error) {
	if len(globs) == 0 {
		return nil, nil, nil
	}
	out, err := run(ctx, dir, append([]string{"diff", "--name-status", "--no-renames", base}, g.outside()...)...)
	if err != nil {
		return nil, nil, err
	}
	var restored, added []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		status, file := parts[0], parts[1]
		if !matchAny(globs, file) {
			continue
		}
		if status == "A" {
			added = append(added, file)
			continue
		}
		if status == "D" || status == "M" || strings.HasPrefix(status, "T") {
			if _, err := run(ctx, dir, "checkout", base, "--", file); err != nil {
				return nil, nil, err
			}
			restored = append(restored, file)
		}
	}
	return restored, added, nil
}

func (g *Git) ToBase(ctx context.Context, dir, base string, keep []string) error {
	files, err := g.Changed(ctx, dir, base, "HEAD")
	if err != nil {
		return err
	}
	keepSet := map[string]bool{}
	for _, k := range keep {
		keepSet[k] = true
	}
	for _, f := range files {
		if keepSet[f] {
			continue
		}
		if _, err := run(ctx, dir, "cat-file", "-e", base+":"+f); err != nil {
			if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if _, err := run(ctx, dir, "checkout", base, "--", f); err != nil {
			return err
		}
	}
	return nil
}

func (g *Git) Reset(ctx context.Context, dir string) error {
	if _, err := run(ctx, dir, "reset", "--quiet", "--hard", "HEAD"); err != nil {
		return err
	}
	if _, err := run(ctx, dir, "clean", "-fdq", "-e", "node_modules"); err != nil {
		return err
	}
	// reset brings .raisq/ back from the commit; it never stays in a checkout.
	return os.RemoveAll(filepath.Join(dir, ".raisq"))
}

func (g *Git) Remove(dir string) error {
	os.Remove(dir + ".gitsum")
	return os.RemoveAll(dir)
}

func matchAny(globs []string, file string) bool {
	for _, g := range globs {
		if Match(g, file) {
			return true
		}
	}
	return false
}

// Match supports ** across directories, which filepath.Match doesn't.
func Match(pattern, name string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchParts(p, n []string) bool {
	for len(p) > 0 {
		if p[0] == "**" {
			if len(p) == 1 {
				return true
			}
			for i := 0; i <= len(n); i++ {
				if matchParts(p[1:], n[i:]) {
					return true
				}
			}
			return false
		}
		if len(n) == 0 {
			return false
		}
		if ok, _ := filepath.Match(p[0], n[0]); !ok {
			return false
		}
		p, n = p[1:], n[1:]
	}
	return len(n) == 0
}
