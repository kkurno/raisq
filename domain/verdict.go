package domain

import (
	"strings"
	"time"
)

type Verdict string

const (
	Pass      Verdict = "pass"
	Fail      Verdict = "fail"
	Unsure    Verdict = "unsure"
	NotJudged Verdict = "n/a"
)

// Claim is one fact in the output a user would see, as the critic quoted it.
type Claim struct {
	Quote string `json:"quote"`
}

// Provenance is the critic's answer for one claim.
type Provenance struct {
	Claim     string `json:"claim"`
	FactID    string `json:"fact_id,omitempty"` // empty: not in spec
	Agrees    bool   `json:"agrees"`
	FactQuote string `json:"fact_quote,omitempty"`
	// Differs is what the critic says is different, when it disagrees.
	Differs string `json:"differs,omitempty"`
}

// CoverageAnswer is the critic's answer for one acceptance criterion.
type CoverageAnswer struct {
	CriterionID string `json:"criterion_id"`
	Quote       string `json:"quote,omitempty"` // empty: missing
}

type RubricAnswerKind string

const (
	Yes   RubricAnswerKind = "yes"
	No    RubricAnswerKind = "no"
	Maybe RubricAnswerKind = "unsure"
	NA    RubricAnswerKind = "n/a"
)

type RubricAnswer struct {
	RubricID  string           `json:"rubric_id"`
	Answer    RubricAnswerKind `json:"answer"`
	Quote     string           `json:"quote,omitempty"`
	LookedFor string           `json:"looked_for,omitempty"`
}

// Rubric is one standing question.
type Rubric struct {
	ID        string           `yaml:"id" json:"id"`
	Ask       string           `yaml:"ask" json:"ask"`
	FailIf    RubricAnswerKind `yaml:"fail_if" json:"fail_if"`
	Lesson    string           `yaml:"lesson,omitempty" json:"lesson,omitempty"`
	Retired   bool             `yaml:"retired,omitempty" json:"retired,omitempty"`
	Overruled int              `yaml:"overruled,omitempty" json:"overruled,omitempty"`
}

// Judgement is one decided item for the review pack.
type Judgement struct {
	Kind    string  `json:"kind"` // claim, criterion, rubric
	Subject string  `json:"subject"`
	Verdict Verdict `json:"verdict"`
	Why     string  `json:"why"`
	Quote   string  `json:"quote,omitempty"`
	FactID  string  `json:"fact_id,omitempty"`
	FactTag Tag     `json:"fact_tag,omitempty"`
}

// Evidence is everything a verdict is decided from. Output and the spec text
// are what quotes must be found in.
type Evidence struct {
	Output     string
	Facts      Facts
	Spec       TaskSpec
	Rubrics    []Rubric
	Claims     []Claim
	Provenance []Provenance
	Coverage   []CoverageAnswer
	Answers    []RubricAnswer
	Now        time.Time
	Periods    map[string]time.Duration
	// Unanswered is claims the critic gave no provenance for: unsure.
	Unanswered []string
	// QuoteRetried marks answers whose quote failed once and were asked again.
	QuoteRetried map[string]bool
}

// Decide applies the verdict rules. No AI is involved past this point.
func Decide(e Evidence) []Judgement {
	var out []Judgement
	inOutput := func(q string) bool { return q != "" && contains(e.Output, q) }

	byClaim := map[string]Provenance{}
	for _, p := range e.Provenance {
		byClaim[p.Claim] = p
	}
	unanswered := map[string]bool{}
	for _, q := range e.Unanswered {
		unanswered[q] = true
	}
	for _, c := range e.Claims {
		j := Judgement{Kind: "claim", Subject: c.Quote, Quote: c.Quote}
		p, answered := byClaim[c.Quote]
		switch {
		case !inOutput(c.Quote):
			j.Verdict, j.Why = Unsure, "the critic quoted text that isn't in the output"
		case unanswered[c.Quote]:
			j.Verdict, j.Why = Unsure, "the critic didn't say which fact backs this"
		case !answered || p.FactID == "":
			j.Verdict, j.Why = Fail, "no fact backs this claim: it was invented"
		default:
			f, ok := e.Facts.Get(p.FactID)
			j.FactID, j.FactTag = p.FactID, f.Tag
			switch {
			case !ok:
				j.Verdict, j.Why = Unsure, "the critic cited a fact that doesn't exist"
			case f.Tag == Gap:
				j.FactTag = Gap
				j.Verdict, j.Why = Fail, "this is a known gap and should have been left out"
			case p.FactQuote != "" && !contains(f.Text, p.FactQuote):
				j.Verdict, j.Why = Unsure, "the critic quoted text that isn't in the fact"
			case !p.Agrees:
				j.FactTag = f.Tag
				j.Verdict, j.Why = Fail, "disagrees with the fact: "+f.Text
				if p.Differs != "" {
					j.Why += " (" + p.Differs + ")"
				}
			default:
				tag := f.EffectiveTag(e.Now, e.Periods)
				j.FactTag = tag
				if tag == Confirmed {
					j.Verdict, j.Why = Pass, "backed by a confirmed fact"
				} else if f.Tag == Confirmed {
					j.Verdict, j.Why = Unsure, "rests on a confirmed fact past its recheck date"
				} else {
					j.Verdict, j.Why = Unsure, "rests on a guessed fact"
				}
			}
		}
		out = append(out, j)
	}

	byCriterion := map[string]CoverageAnswer{}
	for _, c := range e.Coverage {
		byCriterion[c.CriterionID] = c
	}
	for _, cr := range e.Spec.Criteria {
		j := Judgement{Kind: "criterion", Subject: cr.ID + ": " + cr.Text}
		a := byCriterion[cr.ID]
		j.Quote = a.Quote
		switch {
		case a.Quote == "":
			j.Verdict, j.Why = Fail, "the output doesn't show this criterion"
		case !inOutput(a.Quote):
			j.Verdict, j.Why = Unsure, "the critic quoted text that isn't in the output"
		default:
			j.Verdict, j.Why = Pass, "shown in the output"
		}
		out = append(out, j)
	}

	byRubric := map[string]RubricAnswer{}
	for _, a := range e.Answers {
		byRubric[a.RubricID] = a
	}
	for _, r := range e.Rubrics {
		if r.Retired {
			continue
		}
		j := Judgement{Kind: "rubric", Subject: r.ID + ": " + r.Ask}
		a, ok := byRubric[r.ID]
		j.Quote = a.Quote
		switch {
		case !ok:
			j.Verdict, j.Why = Unsure, "the critic didn't answer"
		case a.Answer == NA:
			j.Verdict, j.Why = NotJudged, "nothing to look at: "+a.LookedFor
		case a.Answer == Maybe:
			j.Verdict, j.Why = Unsure, "the critic was unsure"
			if a.LookedFor != "" {
				j.Why += ": " + a.LookedFor
			}
		case a.Quote != "" && !inOutput(a.Quote):
			j.Verdict, j.Why = Unsure, "the critic quoted text that isn't in the output"
		case a.Answer == r.FailIf:
			j.Verdict, j.Why = Fail, "answered "+string(a.Answer)
		default:
			j.Verdict, j.Why = Pass, "answered "+string(a.Answer)
		}
		out = append(out, j)
	}
	return out
}

// Overall: any fail fails the attempt. Unsure items go to the review pack but
// don't send the task back, because only a person can settle them.
func Overall(js []Judgement) Verdict {
	v := Pass
	for _, j := range js {
		if j.Verdict == Fail {
			return Fail
		}
		if j.Verdict == Unsure {
			v = Unsure
		}
	}
	return v
}

// ReliedOn is the facts the critic cited: what a later fact change affects.
func ReliedOn(js []Judgement) []string {
	seen := map[string]bool{}
	var ids []string
	for _, j := range js {
		if j.FactID != "" && !seen[j.FactID] {
			seen[j.FactID] = true
			ids = append(ids, j.FactID)
		}
	}
	return ids
}

// contains matches a quote ignoring differences in whitespace, which models
// often normalise.
func contains(haystack, quote string) bool {
	return strings.Contains(squash(haystack), squash(quote))
}

func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// QuoteFound is the quote check, exported for the use case's ask-again step.
func QuoteFound(haystack, quote string) bool { return quote != "" && contains(haystack, quote) }
