package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kkurno/raisq/domain"
)

func TestFactsRevisions(t *testing.T) {
	s := New(t.TempDir())
	must(t, s.SaveFact(domain.Fact{ID: "business/phone", Text: "02-123", Tag: domain.Confirmed}, ""))
	r1, err := s.PinFacts()
	must(t, err)
	again, err := s.PinFacts()
	must(t, err)
	if r1.Revision != again.Revision {
		t.Fatal("unchanged facts should pin to the same revision")
	}

	must(t, s.SaveFact(domain.Fact{ID: "business/phone", Text: "02-999", Tag: domain.Confirmed}, ""))
	r2, err := s.PinFacts()
	must(t, err)
	if r2.Revision == r1.Revision {
		t.Fatal("a change should make a new revision")
	}

	old, err := s.FactsAt(r1.Revision)
	must(t, err)
	f, _ := old.Get("business/phone")
	if f.Text != "02-123" || f.Topic != "business" {
		t.Fatalf("old revision should keep the old text: %+v", f)
	}
}

func TestHandEditGetsNewRevision(t *testing.T) {
	s := New(t.TempDir())
	must(t, s.SaveFact(domain.Fact{ID: "a/b", Text: "x", Tag: domain.Inferred}, ""))
	r1, _ := s.PinFacts()
	p := filepath.Join(s.Root(), "facts", "a.yaml")
	data, _ := os.ReadFile(p)
	must(t, os.WriteFile(p, append(data, []byte("- id: a/c\n  text: y\n  tag: inferred\n")...), 0o644))
	r2, _ := s.PinFacts()
	if r1.Revision == r2.Revision {
		t.Fatal("hand edits must change the revision")
	}
}

func TestTopicsFromSubfolders(t *testing.T) {
	s := New(t.TempDir())
	must(t, s.SaveFact(domain.Fact{ID: "orders/radius", Text: "5 km", Tag: domain.Confirmed}, "shop/orders"))
	facts, err := s.LoadFacts()
	must(t, err)
	if facts.Items[0].Topic != "shop/orders" {
		t.Fatalf("topic: %q", facts.Items[0].Topic)
	}
}

func TestRecheckOverride(t *testing.T) {
	s := New(t.TempDir())
	p := filepath.Join(s.Root(), "facts", "a.yaml")
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte("- id: a/x\n  text: x\n  tag: confirmed\n  recheck: 2w\n"), 0o644))
	facts, err := s.LoadFacts()
	must(t, err)
	if facts.Items[0].Recheck != 14*day {
		t.Fatal("recheck override not parsed")
	}
}

func TestSpecAndState(t *testing.T) {
	s := New(t.TempDir())
	spec := domain.TaskSpec{Name: "t1", Title: "T", Criteria: []domain.Criterion{{ID: "c1", Text: "x"}}}
	must(t, s.SaveSpec(spec))
	rev, err := s.PinSpec("t1")
	must(t, err)
	got, err := s.SpecAt("t1", rev)
	must(t, err)
	if got.Criteria[0].Text != "x" {
		t.Fatal("pinned spec lost")
	}
	must(t, s.SaveState("t1", domain.TaskState{State: domain.Queued, Added: "now"}))
	st, err := s.LoadState("t1")
	must(t, err)
	if st.State != domain.Queued {
		t.Fatal("state lost")
	}
	tasks, _ := s.Tasks()
	if len(tasks) != 1 {
		t.Fatal("task not listed")
	}
}

func TestRunFiles(t *testing.T) {
	s := New(t.TempDir())
	run, err := s.NewRun("t1")
	must(t, err)
	must(t, s.SaveRunFile("t1", run, "x.json", map[string]int{"a": 1}))
	must(t, s.SaveRunFile("t1", run, "x.md", "hello"))
	var m map[string]int
	must(t, s.LoadRunFile("t1", run, "x.json", &m))
	var md string
	must(t, s.LoadRunFile("t1", run, "x.md", &md))
	if m["a"] != 1 || md != "hello" {
		t.Fatal("run files lost")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
