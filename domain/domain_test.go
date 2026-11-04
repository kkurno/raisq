package domain

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func bakeryFacts() Facts {
	return Facts{Revision: "r1", Items: []Fact{
		{ID: "business/phone", Text: "02-123-4567", Kind: "contact", Tag: Confirmed, SaidOn: "2026-11-03", Topic: "business"},
		{ID: "orders/radius", Text: "Delivery is offered within 5 km", Kind: "terms", Tag: Confirmed, SaidOn: "2026-11-03", Topic: "orders"},
		{ID: "orders/hours", Text: "Tue-Sun 8:00-18:00", Kind: "hours", Tag: Inferred, Topic: "orders"},
		{ID: "orders/minimum", Tag: Gap, Topic: "orders"},
		{ID: "footer/phone", See: "business/phone", Tag: Confirmed, Topic: "footer"},
	}}
}

func spec() TaskSpec {
	return TaskSpec{Name: "cake-orders", Criteria: []Criterion{{ID: "c1", Text: "Customers can pick a delivery date"}}}
}

func decideOne(t *testing.T, output string, p Provenance) Judgement {
	t.Helper()
	js := Decide(Evidence{
		Output: output, Facts: bakeryFacts(), Now: now,
		Claims:     []Claim{{Quote: p.Claim}},
		Provenance: []Provenance{p},
	})
	return js[0]
}

func TestClaimVerdicts(t *testing.T) {
	out := "Call 02-123-4567. We deliver within 5 km. Open Tue-Sun 8:00-18:00. Minimum order 500 baht."
	cases := []struct {
		name string
		p    Provenance
		want Verdict
	}{
		{"confirmed agrees", Provenance{Claim: "Call 02-123-4567", FactID: "business/phone", Agrees: true}, Pass},
		{"see resolves", Provenance{Claim: "Call 02-123-4567", FactID: "footer/phone", Agrees: true}, Pass},
		{"disagrees", Provenance{Claim: "We deliver within 5 km", FactID: "orders/radius", Agrees: false}, Fail},
		{"inferred", Provenance{Claim: "Open Tue-Sun 8:00-18:00", FactID: "orders/hours", Agrees: true}, Unsure},
		{"not in spec", Provenance{Claim: "Minimum order 500 baht"}, Fail},
		{"gap", Provenance{Claim: "Minimum order 500 baht", FactID: "orders/minimum", Agrees: true}, Fail},
		{"quote not in output", Provenance{Claim: "Free delivery", FactID: "orders/radius", Agrees: true}, Unsure},
		{"fact quote not in fact", Provenance{Claim: "We deliver within 5 km", FactID: "orders/radius", Agrees: true, FactQuote: "within 10 km"}, Unsure},
		{"unknown fact", Provenance{Claim: "Call 02-123-4567", FactID: "nope", Agrees: true}, Unsure},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideOne(t, out, c.p).Verdict; got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestExpiredFactIsUnsure(t *testing.T) {
	f := Fact{ID: "x", Text: "x", Kind: "contact", Tag: Confirmed, SaidOn: "2026-01-01"}
	periods := map[string]time.Duration{"contact": 180 * 24 * time.Hour}
	if f.EffectiveTag(now, periods) != Inferred {
		t.Fatal("a confirmed fact past its recheck should count as inferred")
	}
	f.Kind = KindRule
	if f.EffectiveTag(now, periods) != Confirmed {
		t.Fatal("rules never expire")
	}
}

func TestCoverageAndRubrics(t *testing.T) {
	js := Decide(Evidence{
		Output:   "Pick a delivery date: [date]",
		Spec:     TaskSpec{Name: "t", Criteria: []Criterion{{ID: "c1", Text: "date"}, {ID: "c2", Text: "size"}}},
		Coverage: []CoverageAnswer{{CriterionID: "c1", Quote: "Pick a delivery date"}},
		Rubrics: []Rubric{
			{ID: "r1", Ask: "placeholder?", FailIf: Yes},
			{ID: "r2", Ask: "forms show result?", FailIf: No},
			{ID: "r3", Ask: "retired", FailIf: Yes, Retired: true},
		},
		Answers: []RubricAnswer{
			{RubricID: "r1", Answer: Yes, Quote: "[date]"},
			{RubricID: "r2", Answer: NA, LookedFor: "a submit handler"},
		},
	})
	got := map[string]Verdict{}
	for _, j := range js {
		got[j.Subject[:2]] = j.Verdict
	}
	want := map[string]Verdict{"c1": Pass, "c2": Fail, "r1": Fail, "r2": NotJudged}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %s, want %s", k, got[k], v)
		}
	}
	if len(js) != 4 {
		t.Errorf("retired rubric should not be judged, got %d judgements", len(js))
	}
	if Overall(js) != Fail {
		t.Error("a single fail fails the attempt")
	}
}

func TestQuotesIgnoreWhitespace(t *testing.T) {
	if !QuoteFound("We  deliver\nwithin 5 km", "We deliver within 5 km") {
		t.Fatal("whitespace differences shouldn't break a quote")
	}
}

func TestReliedOn(t *testing.T) {
	js := []Judgement{{FactID: "a"}, {FactID: "b"}, {FactID: "a"}, {}}
	if got := strings.Join(ReliedOn(js), ","); got != "a,b" {
		t.Fatalf("got %s", got)
	}
}

func TestJudgeChecks(t *testing.T) {
	results := []CheckResult{
		{Name: "unit", Status: CheckFail, Output: "boom"},
		{Name: "lint", Status: CheckFail},
		{Name: "e2e", Status: CheckBlocked},
		{Name: "build", Status: CheckPass},
	}
	proofs := []TestProof{
		{Name: "date picker", FailsOnOld: true, PassesNew: true},
		{Name: "trivial", FailsOnOld: false, PassesNew: true},
		{Name: "broken", FailsOnOld: true, PassesNew: false},
	}
	o := JudgeChecks(results, Baseline{Failed: map[string]bool{"lint": true}}, proofs)
	if len(o.Failures) != 2 || o.Failures[0].Source != "unit" || o.Failures[1].Source != "test broken" {
		t.Fatalf("failures: %+v", o.Failures)
	}
	if len(o.Old) != 1 || len(o.Blocked) != 1 || len(o.Proven) != 1 || len(o.Unproven) != 1 {
		t.Fatalf("outcome: %+v", o)
	}
}

func TestTaskLifecycle(t *testing.T) {
	s := TaskState{State: Queued}
	var err error
	for _, to := range []State{Running, Ready, Done} {
		if s, err = s.Move(to); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Move(Queued); err == nil {
		t.Fatal("a done task can't be requeued")
	}
}

func TestStuckRule(t *testing.T) {
	a := Handoff{Failures: []Failure{{Source: "unit", What: "check failed"}, {Source: "critic", What: "x"}}}
	b := Handoff{Failures: []Failure{{Source: "critic", What: "x"}, {Source: "unit", What: "check failed"}}}
	if !a.SameAs(b) {
		t.Fatal("same failures in a different order are the same")
	}
	if a.SameAs(Handoff{Failures: []Failure{{Source: "unit", What: "check failed"}}}) {
		t.Fatal("different failures aren't the same")
	}
}

func TestFactsValidate(t *testing.T) {
	if err := bakeryFacts().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := Facts{Items: []Fact{{ID: "a", Tag: Confirmed}}}
	if bad.Validate() == nil {
		t.Fatal("a confirmed fact needs text")
	}
	dangling := Facts{Items: []Fact{{ID: "a", See: "b", Tag: Confirmed}}}
	if dangling.Validate() == nil {
		t.Fatal("see must point at an existing fact")
	}
}

func TestIndex(t *testing.T) {
	idx := bakeryFacts().Index()
	for _, want := range []string{"## orders", "- orders/minimum [gap]: (unknown: leave it out)", "- footer/phone [confirmed]: same as business/phone"} {
		if !strings.Contains(idx, want) {
			t.Errorf("index missing %q:\n%s", want, idx)
		}
	}
}
