package usecase

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/git"
	"github.com/kkurno/raisq/plugin/store"
	"github.com/kkurno/raisq/slot/fake"
)

func TestLessonCheckMustBeProven(t *testing.T) {
	dir, s := project(t)
	bad := gitHead(t, dir)
	write(t, dir, "app.txt", "ok\n")
	sh(t, dir, "git commit -qam fix")
	good := gitHead(t, dir)
	d := Deps{Store: s, Checkout: git.New(dir, t.TempDir()), Isolation: fake.Isolation{}}

	l, err := ProposeLesson(context.Background(), d, "t1", "app said todo", "grep -q ok app.txt", bad, good)
	must(t, err)
	if !l.Proven {
		t.Fatalf("should be proven: %s", l.Why)
	}
	p, err := KeepLesson(d, l)
	must(t, err)
	if filepath.Base(p) != "lesson-app-said-todo.yaml" {
		t.Fatalf("a check lesson is a plugin file: %s", p)
	}

	l, err = ProposeLesson(context.Background(), d, "t1", "trivial", "true", bad, good)
	must(t, err)
	if l.Proven {
		t.Fatal("a check that passes on the mistake proves nothing")
	}
	if _, err := KeepLesson(d, l); err == nil {
		t.Fatal("an unproven check is never kept")
	}
}

func TestOverruleAndRetire(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	must(t, s.SaveRubric(domain.Rubric{ID: "lesson/x", Ask: "x?", FailIf: domain.Yes, Lesson: "task/t"}, "lessons"))
	must(t, s.SaveRubric(domain.Rubric{ID: "web/y", Ask: "y?", FailIf: domain.Yes}, "web"))
	d := Deps{Store: s}
	for i := 0; i < RetireAfter; i++ {
		_, err := Overrule(d, "lesson/x")
		must(t, err)
	}
	if _, err := Overrule(d, "web/y"); err == nil {
		t.Fatal("only lessons are overruled this way")
	}
	rs, _ := ToRetire(d)
	if len(rs) != 1 {
		t.Fatalf("propose retiring after %d overrules, got %v", RetireAfter, rs)
	}
	must(t, Retire(d, "lesson/x"))
	all, _ := s.LoadRubrics()
	for _, r := range all {
		if r.ID == "lesson/x" && !r.Retired {
			t.Fatal("a retired lesson stays on file, marked retired")
		}
	}
}

func TestSpotCheckSkipsParts(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		must(t, s.SaveState(n, domain.TaskState{State: domain.Done}))
	}
	must(t, s.SaveState("a-part-1", domain.TaskState{State: domain.Done, PartOf: "a"}))
	pick, err := SpotCheck(Deps{Store: s}, 0.2, rand.New(rand.NewSource(1)))
	must(t, err)
	if len(pick) != 1 || pick[0] == "a-part-1" {
		t.Fatalf("20%% of 5 whole tasks is 1: %v", pick)
	}
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitOutput(dir, "rev-parse", "HEAD")
	must(t, err)
	return out
}
