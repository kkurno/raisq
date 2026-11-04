// Package builtin holds the plugins that ship inside RaisQ as code.
package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"raisq/slot"
)

// Diff is the built-in git-diff extractor. It returns only what the task
// changed, which is the whole of what the critic should judge.
type Diff struct{}

func (Diff) Name() string { return "git-diff" }

func (Diff) Extract(ctx context.Context, r slot.ExtractRequest) (string, error) {
	d, err := r.Checkout.Diff(ctx, r.Dir, r.Base, r.Head)
	if err != nil {
		return "", err
	}
	return dropLockfiles(d), nil
}

// dropLockfiles keeps lockfile churn out of the critic's context; it carries
// no claims and can be tens of thousands of lines.
func dropLockfiles(diff string) string {
	var b strings.Builder
	skip := false
	for _, line := range strings.SplitAfter(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			skip = false
			for _, lf := range []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.sum", "Cargo.lock", "poetry.lock"} {
				if strings.HasSuffix(strings.TrimSpace(line), "/"+lf) {
					skip = true
					b.WriteString(line + "(lockfile changes omitted)\n")
				}
			}
		}
		if !skip {
			b.WriteString(line)
		}
	}
	return b.String()
}

// Markdown is the built-in report: the review pack is already a file in the
// run; this writes a pointer to the latest one per task.
type Markdown struct{ Root string }

func (Markdown) Name() string { return "markdown" }

func (m Markdown) Deliver(ctx context.Context, it slot.ReportItem) error {
	if it.Task == "" {
		return nil
	}
	dir := filepath.Join(m.Root, "tasks", it.Task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review.md"), []byte(it.Markdown), 0o644)
}
