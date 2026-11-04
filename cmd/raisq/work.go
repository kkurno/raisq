package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/registry"
	"github.com/kkurno/raisq/usecase"
)

func open() (*registry.Project, error) { return registry.Open(wd()) }

func cmdTask(ctx context.Context, args []string) error {
	fs := flags("task")
	from := fs.String("from", "", "an issue id a spec source plugin can fetch, such as SHOP-123")
	specFile := fs.String("spec", "", "a spec tool's file, such as Spec Kit's spec.md or an OpenSpec change, to convert")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 && *from == "" && *specFile == "" {
		return usageErr{`raisq task "what you want built"  ·  --from SHOP-123  ·  --spec specs/001/spec.md`}
	}
	proj, err := open()
	if err != nil {
		return err
	}
	d, err := proj.Deps("", true)
	if err != nil {
		return err
	}
	if *specFile != "" {
		text, err := os.ReadFile(*specFile)
		if err != nil {
			return err
		}
		name := strings.Join(pos, " ")
		if name == "" {
			name = specName(*specFile)
		}
		spec, facts, err := usecase.Convert(ctx, d, name, *specFile, string(text))
		if err != nil {
			return err
		}
		if spec, err = usecase.AddConverted(d, spec, facts); err != nil {
			return err
		}
		logf("Queued: %s (%d acceptance criteria) from %s", spec.Name, len(spec.Criteria), *specFile)
		for _, c := range spec.Criteria {
			logf("  %s  %s", c.ID, c.Text)
		}
		for _, f := range facts {
			logf("  fact %s, inferred until confirmed: %s", f.ID, f.Text)
		}
		return nil
	}
	request := strings.Join(pos, " ")
	outside := ""
	if *from != "" {
		var found bool
		for _, s := range d.Sources {
			if s.Matches(*from) {
				if outside, err = s.Fetch(ctx, *from); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no spec source plugin matches %s", *from)
		}
		if request == "" {
			request = "Do what " + *from + " describes."
		}
	}
	d.SplitOver = proj.Config.SplitOver
	spec, err := usecase.AddTask(ctx, d, request, outside, newTTY())
	if err != nil {
		return err
	}
	if st, err := proj.Store.LoadState(spec.Name); err == nil && st.CheckOnly {
		logf("Queued: %s, split into %d parts that run in order, then the whole is checked together", spec.Name, len(st.After))
		return nil
	}
	logf("Queued: %s (%d acceptance criteria)", spec.Name, len(spec.Criteria))
	for _, c := range spec.Criteria {
		logf("  %s  %s", c.ID, c.Text)
	}
	return nil
}

func cmdRun(ctx context.Context, args []string) error {
	fs := flags("run")
	check := fs.Bool("check", false, "manual mode: you made the change on the task's branch; RaisQ verifies it")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	proj, err := open()
	if err != nil {
		return err
	}
	task := ""
	if len(pos) > 0 {
		task = pos[0]
	} else if task, err = next(proj); err != nil {
		return err
	}
	if *check {
		if err := manualReady(ctx, proj, task); err != nil {
			return err
		}
	}
	return runOne(ctx, proj, task, *check, true)
}

// runOne runs a task. watched means a person is at the terminal, so the
// builder's questions are asked right away instead of parking the task.
func runOne(ctx context.Context, proj *registry.Project, task string, manual, watched bool) error {
	d, err := proj.Deps(task, manual)
	if err != nil {
		return err
	}
	d.Log = logf
	if watched {
		d.Ask = newTTY()
	}
	d.Reload = func() (usecase.Deps, error) {
		fresh, err := registry.Open(proj.Dir)
		if err != nil {
			return usecase.Deps{}, err
		}
		return fresh.Deps(task, manual)
	}
	if err := d.Isolation.Available(ctx); err != nil {
		return fmt.Errorf("%w. Install it, or use `raisq run --check` to build by hand", err)
	}
	_, err = usecase.Run(ctx, d, task)
	if errors.Is(err, usecase.ErrStopped) {
		return err
	}
	if err != nil && !errors.Is(err, usecase.ErrBreach) {
		return err
	}
	if st, _ := proj.Store.LoadState(task); st.State == domain.Ready {
		logf("Review: raisq review %s", task)
	}
	return err
}

// manualReady puts a queued task's state where manual mode expects: the person
// has made the change on raisq/<task> in the project.
func manualReady(ctx context.Context, proj *registry.Project, task string) error {
	st, err := proj.Store.LoadState(task)
	if err != nil {
		return fmt.Errorf("no task %s", task)
	}
	branch := "raisq/" + task
	if _, err := proj.Git.BranchHead(ctx, branch); err != nil {
		head, _ := proj.Git.Head(ctx)
		return fmt.Errorf("make the change on branch %s first:\n    git switch -c %s %s\n  then commit, and run `raisq run --check %s`", branch, branch, short(head), task)
	}
	if st.Base == "" {
		// Before the task is where the branch left main.
		base, err := gitOut(proj.Dir, "merge-base", "HEAD", branch)
		if err != nil {
			return err
		}
		st.Base = base
		return proj.Store.SaveState(task, st)
	}
	return nil
}

func next(proj *registry.Project) (string, error) {
	d, err := proj.Deps("", true)
	if err != nil {
		return "", err
	}
	return usecase.Next(d)
}

func cmdBatch(ctx context.Context, args []string) error {
	fs := flags("batch")
	limit := fs.Duration("for", 0, "stop starting new tasks after this long (default from raisq.yaml, else 8h)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	proj, err := open()
	if err != nil {
		return err
	}
	if *limit == 0 {
		*limit = proj.BatchLimit()
	}
	// Check every tool before the first task, so it doesn't fail at task 7.
	if problems := doctor(ctx, proj); len(problems) > 0 {
		for _, p := range problems {
			logf("✗ %s", p)
		}
		return fmt.Errorf("fix these before a batch")
	}
	if d, err := proj.Deps("", true); err == nil {
		if n, err := usecase.Prune(d, proj.Keep()); err == nil && n > 0 {
			logf("Removed %d run folder(s) older than keep_artifacts.", n)
		}
	}
	stopAt := time.Now().Add(*limit)
	var ready, parked []string
	for time.Now().Before(stopAt) {
		if ctx.Err() != nil {
			logf("Stopped. `raisq batch` resumes with the queue as it is.")
			break
		}
		task, err := next(proj)
		if err != nil {
			if !errors.Is(err, usecase.ErrEmpty) {
				logf("%v", err)
			}
			break
		}
		fmt.Printf("\n▸ %s\n", task)
		err = runOne(ctx, proj, task, false, false)
		if errors.Is(err, usecase.ErrBreach) {
			logf("Audit breach: the setup isn't safe, so the batch stops.")
			break
		}
		if errors.Is(err, usecase.ErrStopped) {
			st, _ := proj.Store.LoadState(task)
			logf("%s. %s is back in the queue; `raisq batch` resumes from it.", st.Note, task)
			break
		}
		if err != nil {
			logf("%s: %v", task, err)
		}
		switch st, _ := proj.Store.LoadState(task); st.State {
		case domain.Ready:
			ready = append(ready, task)
		case domain.Parked:
			parked = append(parked, fmt.Sprintf("%s: %s: %s", task, st.Reason, st.Note))
		}
	}
	report := batchReport(ready, parked)
	fmt.Print("\n" + report)
	d, err := proj.Deps("", true)
	if err == nil {
		for _, r := range d.Reports {
			_ = r.Deliver(ctx, slotItem(report))
		}
	}
	return os.WriteFile(proj.Raisq+"/batch-report.md", []byte(report), 0o644)
}

func batchReport(ready, parked []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Batch report, %s\n\n## Ready for review (%d)\n", time.Now().Format("2006-01-02 15:04"), len(ready))
	for _, r := range ready {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	fmt.Fprintf(&b, "\n## Parked (%d)\n", len(parked))
	for _, p := range parked {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	return b.String()
}

func cmdReview(ctx context.Context, args []string) error {
	fs := flags("review")
	done := fs.Bool("done", false, "mark as done: the branch is ready for your normal merge process")
	back := fs.String("send-back", "", "send back, saying what's wrong; it becomes a criterion")
	answer := fs.String("answer", "", "answer a parked task's question and requeue it")
	overrule := fs.String("overrule", "", "a lesson's fail was wrong: give its rubric id, such as lesson/footer-phone")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	proj, err := open()
	if err != nil {
		return err
	}
	d, err := proj.Deps("", true)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return listReviewable(proj)
	}
	task := pos[0]
	if *overrule != "" {
		n, err := usecase.Overrule(d, *overrule)
		if err != nil {
			return err
		}
		logf("Recorded: %s overruled %d time(s).", *overrule, n)
		if n >= usecase.RetireAfter {
			logf("That's enough to retire it: `raisq learn` will propose it.")
		}
	}
	switch {
	case *done:
		if err := usecase.Review(ctx, d, task, "done", ""); err != nil {
			return err
		}
		logf("Done. Branch raisq/%s is yours to merge the usual way.", task)
	case *back != "":
		if err := usecase.Review(ctx, d, task, "send-back", *back); err != nil {
			return err
		}
		logf("Sent back to the queue with: %s", *back)
		logf("If this mistake could happen again, `raisq learn %s %q` makes it a check.", task, *back)
	case *answer != "":
		if err := usecase.Review(ctx, d, task, "answer", *answer); err != nil {
			return err
		}
		logf("Requeued with the answer.")
	case *overrule != "":
		logf("The lesson still runs until it's retired. To run the task again: raisq review %s --send-back \"why\"", task)
	default:
		data, err := os.ReadFile(proj.Raisq + "/tasks/" + task + "/review.md")
		if err != nil {
			return fmt.Errorf("no review pack for %s yet", task)
		}
		fmt.Print(string(data))
	}
	return nil
}

func listReviewable(proj *registry.Project) error {
	names, _ := proj.Store.Tasks()
	n := 0
	for _, t := range names {
		st, err := proj.Store.LoadState(t)
		if err == nil && (st.State == domain.Ready || st.State == domain.Parked) {
			fmt.Printf("  %-7s %s  %s\n", st.State, t, st.Note)
			n++
		}
	}
	if n == 0 {
		fmt.Println("  Nothing needs a person.")
	} else {
		fmt.Println("\n  raisq review <task> to read one")
	}
	return nil
}

func cmdStatus(ctx context.Context, args []string) error {
	proj, err := open()
	if err != nil {
		return err
	}
	names, err := proj.Store.Tasks()
	if err != nil {
		return err
	}
	type row struct{ state, name, note, added string }
	var rows []row
	for _, t := range names {
		st, err := proj.Store.LoadState(t)
		if err != nil {
			continue
		}
		note := st.Note
		if st.Reason != "" {
			note = string(st.Reason) + ": " + note
		}
		rows = append(rows, row{string(st.State), t, note, st.Added})
	}
	order := map[string]int{"running": 0, "ready": 1, "parked": 2, "queued": 3, "done": 4}
	sort.Slice(rows, func(i, j int) bool {
		if order[rows[i].state] != order[rows[j].state] {
			return order[rows[i].state] < order[rows[j].state]
		}
		return rows[i].added < rows[j].added
	})
	if len(rows) == 0 {
		fmt.Println("  No tasks. raisq task \"...\" adds one.")
	}
	for _, r := range rows {
		fmt.Printf("  %-8s %-28s %s\n", r.state, r.name, r.note)
	}
	facts, err := proj.Store.LoadFacts()
	if err == nil {
		now := time.Now()
		for _, f := range facts.Items {
			if f.Tag == domain.Confirmed && f.EffectiveTag(now, proj.Store.Periods()) != domain.Confirmed {
				fmt.Printf("  due      fact %s: confirmed %s, past its recheck\n", f.ID, f.SaidOn)
			}
		}
	}
	return nil
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// specName is a task name from a spec tool's path: specs/001-cake-orders/spec.md
// gives 001-cake-orders.
func specName(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	if dir == "." || dir == "/" || dir == "specs" || dir == "changes" {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return dir
}
