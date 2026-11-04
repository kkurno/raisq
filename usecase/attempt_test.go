package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/git"
	"github.com/kkurno/raisq/plugin/store"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/slot/fake"
)

type diffExtractor struct{}

func (diffExtractor) Name() string { return "git-diff" }
func (diffExtractor) Extract(ctx context.Context, r slot.ExtractRequest) (string, error) {
	return r.Checkout.Diff(ctx, r.Dir, r.Base, r.Head)
}

// project makes a repo whose test passes when app.txt says "ok".
func project(t *testing.T) (string, *store.Files) {
	t.Helper()
	dir := t.TempDir()
	sh(t, dir, "git init -q -b main && git config user.email t@t && git config user.name t")
	write(t, dir, "app.txt", "todo\n")
	write(t, dir, "test/check.sh", "grep -q ok app.txt\n")
	write(t, dir, "test/old.sh", "true\n")
	sh(t, dir, "git add -A && git commit -qm init")
	s := store.New(filepath.Join(dir, ".raisq"))
	must(t, s.SaveFact(domain.Fact{ID: "business/phone", Text: "02-123-4567", Kind: "contact", Tag: domain.Confirmed, SaidOn: "2026-11-03"}, ""))
	must(t, s.SaveSpec(domain.TaskSpec{Name: "t1", Title: "Make it ok", Criteria: []domain.Criterion{{ID: "c1", Text: "app says ok"}}}))
	must(t, s.SaveState("t1", domain.TaskState{State: domain.Queued, Added: "now"}))
	return dir, s
}

func deps(t *testing.T, dir string, s *store.Files, b *fake.Builder, m *fake.Model) Deps {
	return Deps{
		Store: s, Checkout: git.New(dir, t.TempDir()), Isolation: fake.Isolation{},
		Builder: b, Model: m,
		Checks: []slot.Check{fake.Check{CheckName: "tests", Cmd: "for f in test/*.sh; do sh $f || exit 1; done",
			TestGlob: []string{"test/**"}, OneCmd: "sh {file}"}},
		Extractors: []slot.Extractor{diffExtractor{}},
		Attempts:   3,
		Now:        func() time.Time { return time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC) },
	}
}

// critic answers the read call from the diff, and backs the phone claim.
func critic(claims ...string) *fake.Model {
	return &fake.Model{Fn: func(p string) string {
		if strings.Contains(p, `"provenance"`) {
			var prov []map[string]any
			for _, line := range strings.Split(p[strings.Index(p, "## Claims"):], "\n") {
				var n int
				var c string
				if _, err := fmt.Sscanf(line, "%d.", &n); err != nil {
					continue
				}
				c = line[strings.Index(line, ". ")+2:]
				if strings.Contains(c, "02-123-4567") {
					prov = append(prov, map[string]any{"n": n, "fact_id": "business/phone", "agrees": true, "fact_quote": "02-123-4567"})
				} else {
					prov = append(prov, map[string]any{"n": n, "fact_id": ""})
				}
			}
			return mustJSON(map[string]any{"provenance": prov})
		}
		cov := []domain.CoverageAnswer{{CriterionID: "c1"}}
		if strings.Contains(p, "+ok") {
			cov[0].Quote = "+ok"
		}
		var cl []domain.Claim
		for _, c := range claims {
			if strings.Contains(p, c) {
				cl = append(cl, domain.Claim{Quote: c})
			}
		}
		return mustJSON(map[string]any{"claims": cl, "coverage": cov})
	}}
}

func TestFailThenFix(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, prompt string) (domain.Result, []domain.Choice) {
		if n == 1 {
			write(t, wd, "app.txt", "nearly\n")
		} else {
			if !strings.Contains(prompt, "The last attempt (1) failed") {
				t.Error("the retry should get a handoff")
			}
			write(t, wd, "app.txt", "ok\ncall 02-123-4567\n")
		}
		return domain.Result{Status: domain.ResultDone}, []domain.Choice{{What: "plain text"}}
	}}
	rec, err := Run(context.Background(), deps(t, dir, s, b, critic("call 02-123-4567")), "t1")
	must(t, err)
	if rec.Attempts != 2 || rec.Verdict != domain.Pass {
		t.Fatalf("want pass on attempt 2, got %+v", rec)
	}
	if len(rec.ReliedOn) != 1 || rec.ReliedOn[0] != "business/phone" {
		t.Fatalf("relied on: %v", rec.ReliedOn)
	}
	st, _ := s.LoadState("t1")
	if st.State != domain.Ready {
		t.Fatalf("state %s", st.State)
	}
	var pack string
	must(t, s.LoadRunFile("t1", st.LastRun, "review.md", &pack))
	for _, want := range []string{"plain text", "What earlier attempts got wrong", "✓ Passed"} {
		if !strings.Contains(pack, want) {
			t.Errorf("review pack missing %q:\n%s", want, pack)
		}
	}
	if out, _ := exec.Command("git", "-C", dir, "show", "github.com/kkurno/raisq/t1:app.txt").Output(); !strings.Contains(string(out), "ok") {
		t.Fatal("the branch should be in the project")
	}
}

func TestEditedTestChangesNothing(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "test/check.sh", "true\n") // cheat
		write(t, wd, "app.txt", "still todo\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	rec, err := Run(context.Background(), deps(t, dir, s, b, critic()), "t1")
	must(t, err)
	if rec.Verdict == domain.Pass {
		t.Fatal("editing a test must not make the task pass")
	}
}

func TestFabricationFails(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "app.txt", "ok\nfree delivery anywhere\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	d := deps(t, dir, s, b, critic("free delivery anywhere"))
	d.Attempts = 1
	rec, err := Run(context.Background(), d, "t1")
	must(t, err)
	if rec.Verdict != domain.Fail || rec.Parked != domain.OutOfAttempts {
		t.Fatalf("an invented claim must fail: %+v", rec)
	}
}

func TestStuckParksEarly(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "app.txt", "nope "+string(rune('a'+n))+"\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	d := deps(t, dir, s, b, critic())
	d.Attempts = 5
	rec, err := Run(context.Background(), d, "t1")
	must(t, err)
	if rec.Parked != domain.Stuck || rec.Attempts != 2 {
		t.Fatalf("want stuck after 2, got %+v", rec)
	}
}

func TestQuestionParks(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		return domain.Result{Status: domain.ResultQuestion, Question: "same-day orders?"}, nil
	}}
	rec, err := Run(context.Background(), deps(t, dir, s, b, critic()), "t1")
	must(t, err)
	if rec.Parked != domain.Question || rec.Note != "same-day orders?" {
		t.Fatalf("%+v", rec)
	}
}

func TestNewTestMustBeProven(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "app.txt", "ok\n")
		write(t, wd, "test/proven.sh", "grep -q ok app.txt\n")
		write(t, wd, "test/trivial.sh", "true\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	rec, err := Run(context.Background(), deps(t, dir, s, b, critic()), "t1")
	must(t, err)
	var ar AttemptRecord
	must(t, s.LoadRunFile("t1", recRun(t, s), "attempt-1/attempt.json", &ar))
	if strings.Join(ar.Checks.Proven, ",") != "test/proven.sh" || strings.Join(ar.Checks.Unproven, ",") != "test/trivial.sh" {
		t.Fatalf("proven %v unproven %v", ar.Checks.Proven, ar.Checks.Unproven)
	}
	if rec.Verdict != domain.Pass {
		t.Fatalf("an unproven test is reported, not failed: %+v", rec)
	}
}

func TestBaselineSubtractsOldFailures(t *testing.T) {
	dir, s := project(t)
	write(t, dir, "test/broken.sh", "false\n")
	sh(t, dir, "git add -A && git commit -qm broken")
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "app.txt", "ok\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	d := deps(t, dir, s, b, critic())
	d.Checks = []slot.Check{
		fake.Check{CheckName: "broken", Cmd: "sh test/broken.sh"},
		fake.Check{CheckName: "real", Cmd: "sh test/check.sh"},
	}
	rec, err := Run(context.Background(), d, "t1")
	must(t, err)
	if rec.Verdict != domain.Pass {
		t.Fatalf("an old failure must not fail the task: %+v", rec)
	}
}

func recRun(t *testing.T, s *store.Files) string {
	st, err := s.LoadState("t1")
	must(t, err)
	return st.LastRun
}

func sh(t *testing.T, dir, cmd string) {
	t.Helper()
	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v %s", cmd, err, out)
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte(body), 0o644))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestWatchedQuestionIsAskedNotParked(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, prompt string) (domain.Result, []domain.Choice) {
		if !strings.Contains(prompt, "no same-day orders") {
			return domain.Result{Status: domain.ResultQuestion, Question: "same-day?"}, nil
		}
		write(t, wd, "app.txt", "ok\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	m := &fake.Model{Fn: func(p string) string {
		if strings.Contains(p, "Sort each answer") {
			if !strings.Contains(p, "same-day? → no") {
				t.Error("settle should get the builder's question and the answer")
			}
			return `{"criteria":[{"id":"c1","text":"app says ok, with no same-day orders"}],"facts":[]}`
		}
		return critic().Fn(p)
	}}
	d := deps(t, dir, s, b, m)
	d.Ask = &answers{a: []string{"no"}}
	rec, err := Run(context.Background(), d, "t1")
	must(t, err)
	if rec.Parked != "" || rec.Verdict != domain.Pass {
		t.Fatalf("a watched question should be answered and the run go on: %+v", rec)
	}
	if rec.Attempts != 2 {
		t.Fatalf("an answered question doesn't use up an attempt: %d", rec.Attempts)
	}
	spec, _ := s.LoadSpec("t1")
	if !strings.Contains(spec.Criteria[0].Text, "no same-day orders") {
		t.Fatal("the answer belongs in the spec")
	}
	if pinned, _ := s.SpecAt("t1", rec.Spec); pinned.Criteria[0].Text != spec.Criteria[0].Text {
		t.Fatal("the run is judged against the spec with the answer")
	}
}

func TestUnwatchedQuestionParks(t *testing.T) {
	dir, s := project(t)
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		return domain.Result{Status: domain.ResultQuestion, Question: "same-day?"}, nil
	}}
	rec, err := Run(context.Background(), deps(t, dir, s, b, critic()), "t1")
	must(t, err)
	if rec.Parked != domain.Question {
		t.Fatalf("a batch never waits: %+v", rec)
	}
}

func TestSplitRunsPartsThenTheWhole(t *testing.T) {
	dir, s := project(t)
	var crit []domain.Criterion
	for i := 1; i <= 3; i++ {
		crit = append(crit, domain.Criterion{ID: fmt.Sprintf("c%d", i), Text: fmt.Sprintf("part %d", i)})
	}
	d := deps(t, dir, s, nil, critic())
	must(t, split(d, domain.TaskSpec{Name: "big", Title: "Big", Criteria: crit}, 2, "2026-11-04T00:00:00Z"))

	first, err := Next(d)
	must(t, err)
	if first != "big-part-1" {
		t.Fatalf("the first part runs first, got %s", first)
	}
	files := map[string]string{"big-part-1": "ok\n", "big-part-2": "ok\npart two\n"}
	for _, want := range []string{"big-part-1", "big-part-2"} {
		task, err := Next(d)
		must(t, err)
		if task != want {
			t.Fatalf("want %s, got %s", want, task)
		}
		content := files[task]
		d.Builder = &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
			write(t, wd, "app.txt", content)
			return domain.Result{Status: domain.ResultDone}, nil
		}}
		d.Model = coverAll()
		rec, err := Run(context.Background(), d, task)
		must(t, err)
		if st, _ := s.LoadState(task); st.State != domain.Done {
			t.Fatalf("%s should be done once it passes: %+v %+v", task, st, rec)
		}
	}
	task, err := Next(d)
	must(t, err)
	if task != "big" {
		t.Fatalf("the whole runs last, got %s", task)
	}
	d.Model = coverAll()
	rec, err := Run(context.Background(), d, "big")
	must(t, err)
	if rec.Verdict != domain.Pass {
		t.Fatalf("the whole is checked on the parts' branch: %+v", rec)
	}
	st, _ := s.LoadState("big")
	if st.State != domain.Ready {
		t.Fatal("the whole goes to review")
	}
	p1, _ := s.LoadState("big-part-1")
	if p1.Base == "" || p1.Base != st.Base {
		t.Fatal("parts and whole share one base")
	}
}

// coverAll is a critic that finds every criterion in any output.
func coverAll() *fake.Model {
	return &fake.Model{Fn: func(p string) string {
		var cov []domain.CoverageAnswer
		for _, id := range []string{"c1", "c2", "c3"} {
			if strings.Contains(p, "- "+id+":") {
				cov = append(cov, domain.CoverageAnswer{CriterionID: id, Quote: "ok"})
			}
		}
		return mustJSON(map[string]any{"claims": []any{}, "coverage": cov})
	}}
}

func gitOutput(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}

func TestBuilderCantPushOrSeeRaisq(t *testing.T) {
	dir, s := project(t)
	sh(t, dir, "git add -A && git commit -qm raisq-files")
	var pushed, sawRaisq bool
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		c := exec.Command("sh", "-c", "git push origin HEAD 2>&1 || git remote -v")
		c.Dir = wd
		out, err := c.CombinedOutput()
		pushed = err == nil && strings.Contains(string(out), "->")
		_, err = os.Stat(filepath.Join(wd, ".raisq"))
		sawRaisq = err == nil
		write(t, wd, "app.txt", "ok\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	_, err := Run(context.Background(), deps(t, dir, s, b, critic()), "t1")
	must(t, err)
	if pushed {
		t.Fatal("the checkout must have nothing to push to")
	}
	if sawRaisq {
		t.Fatal("the builder must never see .raisq/")
	}
}

func TestEditingCheckConfigChangesNothing(t *testing.T) {
	dir, s := project(t)
	write(t, dir, "check.conf", "strict\n")
	sh(t, dir, "git add -A && git commit -qm conf")
	b := &fake.Builder{Fn: func(n int, wd, _ string) (domain.Result, []domain.Choice) {
		write(t, wd, "check.conf", "lenient\n")
		write(t, wd, "app.txt", "nope\n")
		return domain.Result{Status: domain.ResultDone}, nil
	}}
	d := deps(t, dir, s, b, critic())
	d.Attempts = 1
	d.Checks = []slot.Check{fake.Check{CheckName: "conf", Cmd: `grep -q lenient check.conf || grep -q ok app.txt`, Lock: []string{"check.conf"}}}
	rec, err := Run(context.Background(), d, "t1")
	must(t, err)
	if rec.Verdict == domain.Pass {
		t.Fatal("a check's config is restored from before the task")
	}
}
