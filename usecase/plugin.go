package usecase

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
)

// Trial is what a check did when run once on the project as it is now,
// before it's added: whether it can protect the project.
type Trial struct {
	Status domain.CheckStatus
	Output string
}

// Verdict says what a trial means for the person deciding whether to add it.
func (t Trial) Verdict() string {
	switch t.Status {
	case domain.CheckPass:
		return "passes on the project as it is, so it will catch a task that breaks it"
	case domain.CheckFail:
		return "already fails before any task, so it can't catch anything until that's fixed: it counts as failing before the task"
	default:
		return "is blocked: its tool or something it needs isn't there, so every task would be blocked by it"
	}
}

// TrialChecks runs checks once each in one fresh session on the project's
// current commit, with the providers they need, the way an attempt would.
// One session means dependencies install once however many are tried.
// Nothing in the project changes.
func TrialChecks(ctx context.Context, d Deps, checks []slot.Check) (map[string]Trial, error) {
	out := map[string]Trial{}
	if len(checks) == 0 {
		return out, nil
	}
	head, err := d.Checkout.Head(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := d.Checkout.Prepare(ctx, "trial", "raisq-trial", head)
	if err != nil {
		return nil, err
	}
	defer d.Checkout.Remove(dir)
	outDir := filepath.Join(filepath.Dir(dir), "trial-out-"+short(head))
	defer os.RemoveAll(outDir)
	trial := d
	trial.Checks = checks
	sess, err := d.Isolation.Start(ctx, slot.StartRequest{Checkout: dir, OutDir: outDir, Label: "trial", Ports: providerPorts(trial)})
	if err != nil {
		return nil, err
	}
	defer sess.Stop(ctx)
	blockAll := func(why string) map[string]Trial {
		for _, c := range checks {
			out[c.Name()] = Trial{Status: domain.CheckBlocked, Output: why}
		}
		return out
	}
	if err := install(ctx, sess); err != nil {
		return blockAll(err.Error()), nil
	}
	provs := startProviders(ctx, trial, sess)
	for _, c := range checks {
		r := provs.run(ctx, c, sess)
		out[c.Name()] = Trial{Status: r.Status, Output: r.Output}
	}
	return out, nil
}
