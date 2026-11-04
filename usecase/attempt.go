// Package usecase holds RaisQ's flows. The order of the steps here is where
// the safety is, and nothing outside this package can change it.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
)

// Deps are the plugins a flow uses, chosen by the registry.
type Deps struct {
	Store      slot.Store
	Checkout   slot.Checkout
	Isolation  slot.Isolation
	Builder    slot.Builder
	Model      slot.Model
	Checks     []slot.Check
	Providers  []slot.Provider
	Extractors []slot.Extractor
	Reports    []slot.Report
	Sources    []slot.SpecSource
	// ForAI is every plugin's for_ai block, already rendered.
	ForAI []string
	// Manual means a person builds; RaisQ does everything after.
	Manual   bool
	Attempts int
	// SplitOver is how many criteria a task may have before raisq task
	// proposes splitting it.
	SplitOver int
	// Ask is a person at the terminal in a watched run. A batch leaves it nil,
	// so a question parks the task instead of waiting.
	Ask Asker
	// Reload rereads the plugin files, after a person wrote a provider mid-run.
	Reload func() (Deps, error)
	// Notices go at the top of every review pack: no isolation, or a critic
	// from the same vendor or model as the builder.
	Notices []string
	Now     func() time.Time
	Log     func(format string, args ...any)
}

func (d Deps) log(format string, args ...any) {
	if d.Log != nil {
		d.Log(format, args...)
	}
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// RunRecord is run.json: what a run was for and how it ended.
type RunRecord struct {
	Task     string              `json:"task"`
	Facts    string              `json:"facts_revision"`
	Spec     string              `json:"spec_revision"`
	Base     string              `json:"base"`
	Branch   string              `json:"branch"`
	Attempts int                 `json:"attempts"`
	Verdict  domain.Verdict      `json:"verdict,omitempty"`
	Head     string              `json:"head,omitempty"`
	ReliedOn []string            `json:"relied_on,omitempty"`
	Parked   domain.ParkedReason `json:"parked,omitempty"`
	Note     string              `json:"note,omitempty"`
	Started  string              `json:"started"`
	Finished string              `json:"finished,omitempty"`
}

// AttemptRecord is attempt-N/attempt.json.
type AttemptRecord struct {
	N           int                 `json:"n"`
	Head        string              `json:"head"`
	Changed     []string            `json:"changed"`
	Result      domain.Result       `json:"result"`
	Checks      domain.CheckOutcome `json:"checks"`
	Judgements  []domain.Judgement  `json:"judgements"`
	Verdict     domain.Verdict      `json:"verdict"`
	Handoff     *domain.Handoff     `json:"handoff,omitempty"`
	Breach      string              `json:"breach,omitempty"`
	CriticCalls int                 `json:"critic_calls"`
	Reused      int                 `json:"provenance_reused"`
}

var ErrBreach = errors.New("audit breach")

// ErrStopped means the run was stopped by a usage limit or an interrupt. The
// task is queued again, and a batch should stop starting new ones.
var ErrStopped = errors.New("stopped")

type errStop struct{ err error }

func (e errStop) Error() string { return e.err.Error() }

// Run takes one task through the loop. It returns the record of the run.
func Run(ctx context.Context, d Deps, task string) (RunRecord, error) {
	st, err := d.Store.LoadState(task)
	if err != nil {
		return RunRecord{}, err
	}
	if st, err = st.Move(domain.Running); err != nil {
		return RunRecord{}, err
	}
	spec, err := d.Store.LoadSpec(task)
	if err != nil {
		return RunRecord{}, err
	}
	facts, err := d.Store.PinFacts()
	if err != nil {
		return RunRecord{}, err
	}
	specRev, err := d.Store.PinSpec(task)
	if err != nil {
		return RunRecord{}, err
	}
	if st.Base == "" {
		if st.Base, err = d.Checkout.Head(ctx); err != nil {
			return RunRecord{}, err
		}
		shareBase(d, task, st)
	}
	run, err := d.Store.NewRun(task)
	if err != nil {
		return RunRecord{}, err
	}
	st.LastRun = run
	if err := d.Store.SaveState(task, st); err != nil {
		return RunRecord{}, err
	}
	branch := st.Branch
	if branch == "" {
		branch = "raisq/" + task
	}
	if st.CheckOnly {
		d.Manual = true
	}
	rec := RunRecord{Task: task, Facts: facts.Revision, Spec: specRev, Base: st.Base,
		Branch: branch, Started: d.now().UTC().Format(time.RFC3339)}
	d.log("Running %s: %q", task, spec.Title)
	d.log("Spec: %d acceptance criteria · %d facts · facts@%s", len(spec.Criteria), len(facts.Items), facts.Revision)

	rubrics, err := d.Store.LoadRubrics()
	if err != nil {
		return rec, err
	}
	base, err := baseline(ctx, d, st.Base)
	if err != nil {
		return rec, err
	}

	var handoffs []domain.Handoff
	var last *AttemptRecord
	settled := map[string]domain.Provenance{}
	limit := d.Attempts
	if limit <= 0 {
		limit = 3
	}
	// In manual mode nothing changes between attempts, so one pass is all
	// there is: a fail goes back to the person, with the review pack.
	if d.Manual {
		limit = 1
	}
	asked := 0
	for n := 1; n <= limit+asked; n++ {
		var prev *domain.Handoff
		if len(handoffs) > 0 {
			prev = &handoffs[len(handoffs)-1]
		}
		ar, err := attempt(ctx, d, task, run, n, spec, facts, rubrics, base, st.Base, rec.Branch, prev, settled)
		rec.Attempts = n
		_ = d.Store.SaveRunFile(task, run, fmt.Sprintf("attempt-%d/attempt.json", n), ar)
		var stop errStop
		if errors.As(err, &stop) {
			// Out of quota or interrupted: back to the queue, not a failure.
			// The branch keeps whatever earlier attempts committed.
			rec.Note = stop.Error()
			_ = d.Store.SaveRunFile(task, run, "run.json", rec)
			st.State = domain.Queued
			st.Note = "stopped: " + stop.Error()
			_ = d.Store.SaveState(task, st)
			return rec, ErrStopped
		}
		if err != nil {
			if errors.Is(err, ErrBreach) {
				rec.Note = ar.Breach
				return finish(d, task, run, rec, st, domain.Parked, domain.Blocked, "audit breach: "+ar.Breach, nil, err)
			}
			return finish(d, task, run, rec, st, domain.Parked, domain.Blocked, err.Error(), nil, err)
		}
		rec.Head = ar.Head
		last = &ar
		if ar.Result.Status == domain.ResultQuestion || ar.Result.Status == domain.ResultNeedsSomething {
			// Watched: ask now, put the answer in the spec, and try again.
			// Unwatched: park, and the next task starts.
			answered, aErr := askNow(ctx, d, task, &spec, facts, ar.Result)
			if aErr != nil {
				return finish(d, task, run, rec, st, domain.Parked, domain.Blocked, aErr.Error(), &ar, nil)
			}
			if !answered {
				if ar.Result.Status == domain.ResultQuestion {
					return finish(d, task, run, rec, st, domain.Parked, domain.Question, ar.Result.Question, &ar, nil)
				}
				return finish(d, task, run, rec, st, domain.Parked, domain.NeedsSomething, ar.Result.Needs, &ar, nil)
			}
			if ar.Result.Status == domain.ResultNeedsSomething && d.Reload != nil {
				fresh, rErr := d.Reload()
				if rErr != nil {
					return finish(d, task, run, rec, st, domain.Parked, domain.Blocked, rErr.Error(), &ar, nil)
				}
				d.Providers, d.Checks, d.ForAI = fresh.Providers, fresh.Checks, fresh.ForAI
			}
			if specRev, err = d.Store.PinSpec(task); err != nil {
				return rec, err
			}
			if facts, err = d.Store.PinFacts(); err != nil {
				return rec, err
			}
			rec.Spec, rec.Facts = specRev, facts.Revision
			_ = os.RemoveAll(filepath.Join(d.Store.RunDir(task, run), "spec"))
			// A question answered isn't a failed attempt, so it doesn't use one up.
			asked++
			if asked > 3 {
				return finish(d, task, run, rec, st, domain.Parked, domain.Question, "the builder kept asking: the spec needs more before it can be built", &ar, nil)
			}
			continue
		}
		if len(ar.Checks.Blocked) > 0 {
			why := ""
			for _, r := range ar.Checks.Results {
				if r.Status == domain.CheckBlocked {
					why += "\n" + domain.TailLines(r.Output, 3)
				}
			}
			return finish(d, task, run, rec, st, domain.Parked, domain.Blocked, "missing tool for: "+strings.Join(ar.Checks.Blocked, ", ")+why, &ar, nil)
		}
		d.log("attempt %d · %s", n, summary(ar))
		if ar.Verdict != domain.Fail {
			rec.Verdict = ar.Verdict
			rec.ReliedOn = domain.ReliedOn(ar.Judgements)
			if st.PartOf != "" && ar.Verdict == domain.Pass {
				// A part is reviewed with its whole, whose check covers the branch.
				return finish(d, task, run, rec, st, domain.Done, "", "reviewed with "+st.PartOf, &ar, nil)
			}
			return finish(d, task, run, rec, st, domain.Ready, "", "", &ar, nil)
		}
		handoffs = append(handoffs, *ar.Handoff)
		if len(handoffs) >= 2 && handoffs[len(handoffs)-1].SameAs(handoffs[len(handoffs)-2]) {
			rec.Verdict = domain.Fail
			return finish(d, task, run, rec, st, domain.Parked, domain.Stuck, "two attempts failed the same way", &ar, nil)
		}
		for _, j := range ar.Judgements {
			if j.Kind == "claim" && j.Verdict != domain.Unsure {
				settled[j.Quote] = provenanceOf(j)
			}
		}
	}
	rec.Verdict = domain.Fail
	note := fmt.Sprintf("%d attempts used", limit)
	if d.Manual {
		note = "failed the checks or the critic; fix it on the branch and run --check again"
	}
	return finish(d, task, run, rec, st, domain.Parked, domain.OutOfAttempts, note, last, nil)
}

// askNow answers a builder's question or need in a watched run. A question's
// answer is settled like any answer at raisq task: into the criteria the
// critic judges against, or into the facts.
func askNow(ctx context.Context, d Deps, task string, spec *domain.TaskSpec, facts domain.Facts, r domain.Result) (bool, error) {
	if d.Ask == nil {
		return false, nil
	}
	if r.Status == domain.ResultNeedsSomething {
		d.log("The builder needs: %s", r.Needs)
		d.log("Nothing is allowed automatically.")
		a, err := d.Ask.Ask(fmt.Sprintf("Write a provider for it, in .raisq/plugins/ or, to allow it for this task only, .raisq/tasks/%s/plugins/. Then answer done, or deny.", task))
		if err != nil {
			return false, err
		}
		return strings.EqualFold(strings.TrimSpace(a), "done"), nil
	}
	a, err := d.Ask.Ask("The builder asks: " + r.Question)
	if err != nil || strings.TrimSpace(a) == "" {
		return false, err
	}
	if err := settle(ctx, d, spec.Title, spec, spec.RestsOn, []string{r.Question + " → " + a}, facts, d.Ask); err != nil {
		return false, err
	}
	return true, d.Store.SaveSpec(*spec)
}

func provenanceOf(j domain.Judgement) domain.Provenance {
	return domain.Provenance{Claim: j.Quote, FactID: j.FactID, Agrees: j.Verdict == domain.Pass || j.FactTag != "" && j.Verdict == domain.Unsure}
}

func summary(a AttemptRecord) string {
	if a.Breach != "" {
		return "audit breach"
	}
	parts := []string{"built"}
	switch {
	case a.Result.Summary == "made by a person":
		parts = []string{"checked"}
	case a.Result.Status == domain.ResultFailed:
		parts = []string{"builder failed: " + a.Result.Summary}
	}
	if len(a.Checks.Failures) > 0 {
		parts = append(parts, fmt.Sprintf("checks: %d failed", len(a.Checks.Failures)))
	} else {
		parts = append(parts, "checks passed")
	}
	var fail, unsure, claims int
	var first string
	for _, j := range a.Judgements {
		if j.Kind == "claim" {
			claims++
		}
		switch j.Verdict {
		case domain.Fail:
			fail++
			if first == "" {
				first = j.Subject + ": " + j.Why
			}
		case domain.Unsure:
			unsure++
		}
	}
	parts = append(parts, fmt.Sprintf("critic: %d claims checked, %d failed, %d unsure", claims, fail, unsure))
	if first != "" {
		parts = append(parts, first)
	}
	return strings.Join(parts, " · ")
}

func finish(d Deps, task, run string, rec RunRecord, st domain.TaskState, to domain.State,
	reason domain.ParkedReason, note string, last *AttemptRecord, err error) (RunRecord, error) {
	rec.Finished = d.now().UTC().Format(time.RFC3339)
	if to == domain.Parked {
		rec.Parked, rec.Note = reason, note
	}
	if to == domain.Done {
		// Only a part reaches done from a run, through ready.
		if st, err = st.Move(domain.Ready); err != nil {
			return rec, err
		}
	}
	st, mErr := st.Move(to)
	if mErr != nil && err == nil {
		err = mErr
	}
	st.Reason, st.Note = reason, note
	_ = d.Store.SaveState(task, st)
	_ = d.Store.SaveRunFile(task, run, "run.json", rec)
	if last != nil {
		pack := ReviewPack(d, task, run, rec, *last)
		_ = d.Store.SaveRunFile(task, run, "review.md", pack)
		for _, r := range d.Reports {
			if rErr := r.Deliver(context.Background(), slot.ReportItem{Task: task, State: to, Markdown: pack, Branch: rec.Branch}); rErr != nil {
				d.log("report %s: %v (the local record is complete)", r.Name(), rErr)
			}
		}
	}
	switch to {
	case domain.Ready:
		d.log("Ready for review.")
	case domain.Done:
		d.log("Part passed: %s.", note)
	case domain.Parked:
		d.log("Parked: %s: %s", reason, note)
	}
	return rec, err
}

// baseline measures what fails on a base commit, once per commit.
func baseline(ctx context.Context, d Deps, base string) (domain.Baseline, error) {
	if b, ok, err := d.Store.Baseline(base); err != nil || ok {
		return b, err
	}
	b := domain.Baseline{Commit: base, Failed: map[string]bool{}}
	snaps := snapshotters(d)
	if len(d.Checks) == 0 && len(snaps) == 0 {
		return b, d.Store.SaveBaseline(b)
	}
	dir, err := d.Checkout.Prepare(ctx, "baseline-"+short(base), "raisq-baseline", base)
	if err != nil {
		return b, err
	}
	defer d.Checkout.Remove(dir)
	out := filepath.Join(filepath.Dir(dir), "baseline-out-"+short(base))
	defer os.RemoveAll(out)
	sess, err := d.Isolation.Start(ctx, slot.StartRequest{Checkout: dir, OutDir: out, Label: "baseline", Ports: providerPorts(d)})
	if err != nil {
		return b, err
	}
	defer sess.Stop(ctx)
	if err := install(ctx, sess); err != nil {
		return b, err
	}
	env, err := startProviders(ctx, d, sess)
	if err != nil {
		return b, err
	}
	for _, c := range d.Checks {
		r := c.Run(ctx, sess, envFor(c, env))
		if r.Status == domain.CheckFail {
			b.Failed[c.Name()] = true
		}
	}
	// How the output looked before the task, once per base commit, for the
	// checks and extractors that compare against it.
	for name, sn := range snaps {
		pages, err := sn.Snapshot(ctx, sess)
		if err != nil {
			return b, fmt.Errorf("%s at the base: %w", name, err)
		}
		if err := d.Store.SaveSnapshot(base, name, pages); err != nil {
			return b, err
		}
	}
	d.log("Old failures on %s: %d", short(base), len(b.Failed))
	return b, d.Store.SaveBaseline(b)
}

func snapshotters(d Deps) map[string]slot.Snapshotter {
	out := map[string]slot.Snapshotter{}
	for _, e := range d.Extractors {
		if sn, ok := e.(slot.Snapshotter); ok {
			out[e.Name()] = sn
		}
	}
	for _, c := range d.Checks {
		if sn, ok := c.(slot.Snapshotter); ok {
			out[c.Name()] = sn
		}
	}
	return out
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func attempt(ctx context.Context, d Deps, task, run string, n int, spec domain.TaskSpec, facts domain.Facts,
	rubrics []domain.Rubric, base domain.Baseline, baseCommit, branch string, prev *domain.Handoff,
	settled map[string]domain.Provenance) (AttemptRecord, error) {
	ar := AttemptRecord{N: n}
	runDir := d.Store.RunDir(task, run)
	attemptDir := filepath.Join(runDir, fmt.Sprintf("attempt-%d", n))
	outDir := filepath.Join(attemptDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return ar, err
	}
	specDir := filepath.Join(runDir, "spec")
	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		if err := writeSpecView(specDir, spec, facts); err != nil {
			return ar, err
		}
	}

	dir, err := d.Checkout.Prepare(ctx, task, branch, baseCommit)
	if err != nil {
		return ar, err
	}
	defer d.Checkout.Remove(dir)

	var mounts []slot.Mount
	for _, p := range d.Providers {
		if m, ok := p.Mount(); ok && p.For() == slot.ForBuilder {
			mounts = append(mounts, m)
		}
	}
	sess, err := d.Isolation.Start(ctx, slot.StartRequest{Checkout: dir, SpecDir: specDir, OutDir: outDir,
		Mounts: mounts, Ports: providerPorts(d), Label: task})
	if err != nil {
		return ar, err
	}
	defer sess.Stop(ctx)

	// 1-2. Build. In manual mode a person has already made the change on the
	// branch, and the checkout starts from it.
	if !d.Manual {
		prompt := Prompt(d, spec, facts, sess.Paths(), prev)
		_ = d.Store.SaveRunFile(task, run, fmt.Sprintf("attempt-%d/prompt.md", n), prompt)
		if err := d.Builder.Build(ctx, sess, prompt); err != nil {
			if errors.Is(err, slot.ErrUsageLimit) || ctx.Err() != nil {
				return ar, errStop{err}
			}
			d.log("builder: %v", err)
		}
		ar.Result = readResult(outDir)
	} else {
		ar.Result = domain.Result{Status: domain.ResultDone, Summary: "made by a person"}
	}

	// 3. Audit, then commit.
	if err := d.Checkout.AuditGit(ctx, dir); err != nil {
		ar.Breach = err.Error()
		return ar, ErrBreach
	}
	if err := auditMounts(mounts); err != nil {
		ar.Breach = err.Error()
		return ar, ErrBreach
	}
	if ar.Result.Status == domain.ResultQuestion || ar.Result.Status == domain.ResultNeedsSomething {
		return ar, nil
	}
	head, _, err := d.Checkout.Commit(ctx, dir, fmt.Sprintf("raisq: %s, attempt %d", task, n))
	if err != nil {
		return ar, err
	}
	ar.Head = head
	if ar.Changed, err = d.Checkout.Changed(ctx, dir, baseCommit, head); err != nil {
		return ar, err
	}
	if !d.Manual {
		if err := d.Checkout.Bring(ctx, dir, branch); err != nil {
			return ar, err
		}
	}
	if len(ar.Changed) == 0 {
		ar.Verdict = domain.Fail
		ar.Handoff = &domain.Handoff{Attempt: n, Failures: []domain.Failure{{Source: "raisq", What: "no change was made"}}}
		if ar.Result.Status == domain.ResultFailed {
			ar.Handoff.Failures[0].What = "the builder reported failure and made no change: " + ar.Result.Summary
		}
		return ar, nil
	}

	// 4. Checks, on tests restored from before the task. Dependencies the
	// builder didn't install are installed first, so a check can run at all.
	if err := install(ctx, sess); err != nil {
		return ar, err
	}
	env, err := startProviders(ctx, d, sess)
	if err != nil {
		return ar, err
	}
	ar.Checks, err = runChecks(ctx, d, sess, dir, baseCommit, base, env)
	if err != nil {
		return ar, err
	}
	_ = d.Store.SaveRunFile(task, run, fmt.Sprintf("attempt-%d/checks.json", n), ar.Checks)
	if len(ar.Checks.Blocked) > 0 {
		return ar, nil
	}

	// 5. The critic, even when a check failed, so one attempt reports
	// everything wrong with it.
	output, err := extract(ctx, d, dir, baseCommit, head, sess)
	if err != nil {
		return ar, err
	}
	_ = d.Store.SaveRunFile(task, run, fmt.Sprintf("attempt-%d/output.txt", n), output)
	cr, err := Critic{Model: d.Model}.Judge(ctx, CriticInput{Output: output, Spec: spec, Facts: facts, Rubrics: rubrics, Settled: settled})
	ar.CriticCalls, ar.Reused = cr.Calls, cr.Reused
	if err != nil {
		return ar, err
	}
	_ = d.Store.SaveRunFile(task, run, fmt.Sprintf("attempt-%d/critic.json", n), cr)

	// 6. RaisQ decides.
	ar.Judgements = domain.Decide(domain.Evidence{
		Output: output, Facts: facts, Spec: spec, Rubrics: rubrics,
		Claims: cr.Claims, Provenance: cr.Provenance, Coverage: cr.Coverage, Answers: cr.Rubrics,
		Unanswered: cr.Unanswered, Now: d.now(), Periods: d.Store.Periods(),
	})
	ar.Verdict = domain.Overall(ar.Judgements)
	if len(ar.Checks.Failures) > 0 {
		ar.Verdict = domain.Fail
	}
	if ar.Verdict == domain.Fail {
		ar.Handoff = handoff(n, ar)
	}
	return ar, nil
}

func readResult(outDir string) domain.Result {
	data, err := os.ReadFile(filepath.Join(outDir, "result.json"))
	if err != nil {
		return domain.Result{Status: domain.ResultFailed, Summary: "no result.json"}
	}
	var r domain.Result
	if json.Unmarshal(data, &r) != nil || r.Validate() != nil {
		return domain.Result{Status: domain.ResultFailed, Summary: "result.json was not valid"}
	}
	return r
}

func readChoices(outDir string) []domain.Choice {
	var cs []domain.Choice
	data, err := os.ReadFile(filepath.Join(outDir, "choices.json"))
	if err == nil {
		_ = json.Unmarshal(data, &cs)
	}
	return cs
}

func handoff(n int, ar AttemptRecord) *domain.Handoff {
	h := &domain.Handoff{Attempt: n, Changed: ar.Changed}
	h.Failures = append(h.Failures, ar.Checks.Failures...)
	for _, j := range ar.Judgements {
		if j.Verdict != domain.Fail {
			continue
		}
		what := j.Why
		switch {
		case j.Kind == "rubric":
			// The question is what failed; the quote is where.
			what = j.Subject + " " + j.Why
			if j.Quote != "" {
				what += fmt.Sprintf(", at %q", j.Quote)
			}
		case j.Quote != "":
			what = fmt.Sprintf("%q: %s", j.Quote, j.Why)
		}
		h.Failures = append(h.Failures, domain.Failure{Source: "critic " + j.Kind, What: what, Lines: j.Subject})
	}
	return h
}

func providerPorts(d Deps) []int {
	var ports []int
	for _, p := range d.Providers {
		if p.Port() > 0 {
			ports = append(ports, p.Port())
		}
	}
	return ports
}

// install gets dependencies in, the way the builder would, for sessions that
// have no builder: the baseline and manual mode.
func install(ctx context.Context, s slot.Session) error {
	for _, probe := range []struct{ file, cmd string }{
		{"package-lock.json", "[ -d node_modules ] || npm ci --no-audit --no-fund"},
		{"pnpm-lock.yaml", "[ -d node_modules ] || pnpm install --frozen-lockfile"},
		{"yarn.lock", "[ -d node_modules ] || yarn install --frozen-lockfile"},
		{"go.mod", "go mod download"},
	} {
		if _, err := os.Stat(filepath.Join(hostDir(s), probe.file)); err == nil {
			if r, err := s.Exec(ctx, probe.cmd, nil); err != nil || r.Exit != 0 {
				return fmt.Errorf("installing dependencies with `%s` failed:\n%s", probe.cmd, domain.TailLines(r.Output, 10))
			}
		}
	}
	return nil
}

// hostDir is where the checkout is on the host. Sessions mount it at a guest
// path, so the host side is what the request was made with.
func hostDir(s slot.Session) string {
	if h, ok := s.(interface{ HostDir() string }); ok {
		return h.HostDir()
	}
	return s.Paths().Workdir
}

// startProviders starts every provider a check or an extractor asked for,
// after the builder has exited. A need with no provider blocks the task.
func startProviders(ctx context.Context, d Deps, s slot.Session) (map[string]map[string]string, error) {
	want := map[string]bool{}
	for _, c := range d.Checks {
		for _, n := range c.Needs() {
			want[n] = true
		}
	}
	for _, e := range d.Extractors {
		if n, ok := e.(slot.Needer); ok {
			for _, x := range n.Needs() {
				want[x] = true
			}
		}
	}
	env := map[string]map[string]string{}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		var p slot.Provider
		for _, cand := range d.Providers {
			if cand.Name() == n && cand.For() == slot.ForChecks {
				p = cand
			}
		}
		if p == nil {
			return nil, fmt.Errorf("a check needs %q, and no provider supplies it", n)
		}
		e, err := p.Start(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", n, err)
		}
		env[n] = e
	}
	return env, nil
}

func envFor(c slot.Check, env map[string]map[string]string) map[string]string {
	out := map[string]string{}
	for _, n := range c.Needs() {
		for k, v := range env[n] {
			out[k] = v
		}
	}
	return out
}

// runChecks restores every check's locked files from before the task, runs
// every check, then proves each test the builder added.
func runChecks(ctx context.Context, d Deps, s slot.Session, dir, baseCommit string, base domain.Baseline,
	env map[string]map[string]string) (domain.CheckOutcome, error) {
	var locks, tests []string
	for _, c := range d.Checks {
		locks = append(locks, c.Locks()...)
		tests = append(tests, c.Tests()...)
	}
	_, added, err := d.Checkout.Restore(ctx, dir, baseCommit, append(locks, tests...))
	if err != nil {
		return domain.CheckOutcome{}, err
	}
	checks := append([]slot.Check(nil), d.Checks...)
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].Priority() != checks[j].Priority() {
			return checks[i].Priority() < checks[j].Priority()
		}
		return checks[i].Name() < checks[j].Name()
	})
	var results []domain.CheckResult
	for _, c := range checks {
		results = append(results, c.Run(ctx, s, envFor(c, env)))
	}

	// Prove new tests: each must pass on the new code and fail on the old.
	var proofs []domain.TestProof
	for _, file := range added {
		c := checkFor(checks, file)
		if c == nil {
			continue
		}
		newRes, ok := c.RunOne(ctx, s, envFor(c, env), file)
		if !ok {
			continue
		}
		proofs = append(proofs, domain.TestProof{Name: file, PassesNew: newRes.Status == domain.CheckPass})
	}
	if len(proofs) > 0 {
		if err := d.Checkout.ToBase(ctx, dir, baseCommit, added); err != nil {
			return domain.CheckOutcome{}, err
		}
		for i := range proofs {
			c := checkFor(checks, proofs[i].Name)
			oldRes, _ := c.RunOne(ctx, s, envFor(c, env), proofs[i].Name)
			proofs[i].FailsOnOld = oldRes.Status != domain.CheckPass
		}
		if err := d.Checkout.Reset(ctx, dir); err != nil {
			return domain.CheckOutcome{}, err
		}
	}
	return domain.JudgeChecks(results, base, proofs), nil
}

func checkFor(checks []slot.Check, file string) slot.Check {
	for _, c := range checks {
		for _, g := range c.Tests() {
			if globMatch(g, file) {
				return c
			}
		}
	}
	return nil
}

func extract(ctx context.Context, d Deps, dir, base, head string, s slot.Session) (string, error) {
	var parts []string
	for _, e := range d.Extractors {
		var before map[string]string
		if _, ok := e.(slot.Snapshotter); ok {
			b, found, err := d.Store.Snapshot(base, e.Name())
			if err != nil {
				return "", err
			}
			if found {
				before = b
			}
		}
		text, err := e.Extract(ctx, slot.ExtractRequest{Checkout: d.Checkout, Dir: dir, Base: base, Head: head, Session: s, Before: before})
		if err != nil {
			return "", fmt.Errorf("extractor %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(text) != "" {
			parts = append(parts, "### "+e.Name()+"\n"+text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func auditMounts(ms []slot.Mount) error {
	for _, m := range ms {
		if !m.ReadOnly {
			return fmt.Errorf("builder mount %s is not read-only", m.Host)
		}
	}
	return nil
}
