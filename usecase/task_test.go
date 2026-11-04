package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/store"
	"github.com/kkurno/raisq/slot/fake"
)

type answers struct{ a []string }

func (s *answers) Ask(q string) (string, error) {
	v := s.a[0]
	s.a = s.a[1:]
	return v, nil
}
func (s *answers) Confirm(string) (bool, error) { return true, nil }

func TestAddTaskSortsAnswers(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	must(t, s.SaveFact(domain.Fact{ID: "orders/radius", Text: "5 km", Kind: "terms", Tag: domain.Confirmed}, ""))
	m := &fake.Model{Fn: func(p string) string {
		if strings.Contains(p, "Sort each answer") {
			if !strings.Contains(p, "orders/radius [confirmed]: 5 km") {
				t.Error("the settle call should get the facts the draft cited")
			}
			return `{"criteria":[{"id":"c1","text":"Customers pick a date"},{"id":"c2","text":"No same-day orders"}],
			 "facts":[{"id":"orders/minimum","topic":"orders","text":"Minimum order 500 baht","kind":"price","stakeholder":true}]}`
		}
		if strings.Contains(p, "src/") {
			t.Error("the spec writer must not see code")
		}
		return "Here you go:\n```json\n" + `{"name":"cake-orders","title":"Add a cake order form",
		 "criteria":[{"id":"c1","text":"Customers pick a date"}],"rests_on":["orders/radius"],
		 "questions":["Same-day orders?","Minimum order?"]}` + "\n```"
	}}
	d := Deps{Store: s, Model: m}
	spec, err := AddTask(context.Background(), d, "Add a cake order form", "", &answers{a: []string{"no", "500 baht"}})
	must(t, err)
	if spec.Name != "cake-orders" || len(spec.Criteria) != 2 {
		t.Fatalf("spec %+v", spec)
	}
	if len(m.Prompts) != 2 {
		t.Fatalf("want 2 model calls, got %d", len(m.Prompts))
	}
	facts, _ := s.LoadFacts()
	f, ok := facts.Get("orders/minimum")
	if !ok || f.Tag != domain.Inferred || f.Needs != "stakeholder" {
		t.Fatalf("a stakeholder's fact stays inferred: %+v", f)
	}
	st, _ := s.LoadState("cake-orders")
	if st.State != domain.Queued {
		t.Fatal("new task should be queued")
	}
}

func TestFactChangeSendsBackOnlyDependents(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	for _, n := range []string{"uses-phone", "other"} {
		must(t, s.SaveSpec(domain.TaskSpec{Name: n, Criteria: []domain.Criterion{{ID: "c1", Text: "x"}}}))
		run, _ := s.NewRun(n)
		relied := []string{}
		if n == "uses-phone" {
			relied = []string{"business/phone"}
		}
		must(t, s.SaveRunFile(n, run, "run.json", RunRecord{ReliedOn: relied}))
		must(t, s.SaveState(n, domain.TaskState{State: domain.Ready, LastRun: run}))
	}
	back, err := FactChanged(Deps{Store: s}, "business/phone")
	must(t, err)
	if len(back) != 1 || back[0] != "uses-phone" {
		t.Fatalf("sent back %v", back)
	}
	st, _ := s.LoadState("other")
	if st.State != domain.Ready {
		t.Fatal("a task that didn't cite the fact is untouched")
	}
}

func TestDoneRefusesAStaleResult(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	must(t, s.SaveFact(domain.Fact{ID: "a/b", Text: "5 km", Tag: domain.Confirmed}, ""))
	must(t, s.SaveSpec(domain.TaskSpec{Name: "t", Criteria: []domain.Criterion{{ID: "c1", Text: "x"}}}))
	facts, _ := s.PinFacts()
	rev, _ := s.PinSpec("t")
	run, _ := s.NewRun("t")
	must(t, s.SaveRunFile("t", run, "run.json", RunRecord{Facts: facts.Revision, Spec: rev, ReliedOn: []string{"a/b"}}))
	ready := domain.TaskState{State: domain.Ready, LastRun: run}
	must(t, s.SaveState("t", ready))
	d := Deps{Store: s}

	must(t, s.SaveFact(domain.Fact{ID: "a/b", Text: "7 km", Tag: domain.Confirmed}, ""))
	if err := Review(context.Background(), d, "t", "done", ""); err == nil {
		t.Fatal("a fact it relied on changed by hand: done must refuse")
	}
	must(t, s.SaveFact(domain.Fact{ID: "a/b", Text: "5 km", Tag: domain.Confirmed}, ""))
	must(t, s.SaveSpec(domain.TaskSpec{Name: "t", Criteria: []domain.Criterion{{ID: "c1", Text: "y"}}}))
	if err := Review(context.Background(), d, "t", "done", ""); err == nil {
		t.Fatal("the spec changed by hand: done must refuse")
	}
	must(t, s.SaveSpec(domain.TaskSpec{Name: "t", Criteria: []domain.Criterion{{ID: "c1", Text: "x"}}}))
	must(t, Review(context.Background(), d, "t", "done", ""))
}

func TestPruneKeepsLatestAndParked(t *testing.T) {
	s := store.New(filepath.Join(t.TempDir(), ".raisq"))
	old := time.Now().Add(-60 * 24 * time.Hour)
	runs := map[string][]string{}
	for _, task := range []string{"done-task", "parked-task"} {
		for i := 0; i < 2; i++ {
			r, err := s.NewRun(task)
			must(t, err)
			must(t, os.Chtimes(s.RunDir(task, r), old, old))
			runs[task] = append(runs[task], r)
		}
	}
	must(t, s.SaveState("done-task", domain.TaskState{State: domain.Done, LastRun: runs["done-task"][1]}))
	must(t, s.SaveState("parked-task", domain.TaskState{State: domain.Parked, LastRun: runs["parked-task"][1]}))
	n, err := Prune(Deps{Store: s}, 30*24*time.Hour)
	must(t, err)
	if n != 1 {
		t.Fatalf("only the old, not-latest run of a non-parked task goes: removed %d", n)
	}
	if _, err := os.Stat(s.RunDir("done-task", runs["done-task"][1])); err != nil {
		t.Fatal("the latest run stays")
	}
	if _, err := os.Stat(s.RunDir("parked-task", runs["parked-task"][0])); err != nil {
		t.Fatal("a parked task keeps all its evidence")
	}
}
