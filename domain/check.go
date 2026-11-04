package domain

import "strings"

// CheckStatus is what a check plugin returns.
type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckFail    CheckStatus = "fail"
	CheckBlocked CheckStatus = "blocked" // the tool is missing: never a pass
)

type CheckResult struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Score  *float64    `json:"score,omitempty"`
	Output string      `json:"output,omitempty"`
}

// Baseline is what already failed on a base commit. Only new failures count.
type Baseline struct {
	Commit string          `json:"commit"`
	Failed map[string]bool `json:"failed"`
}

// TestProof is a test the builder added, run on the old and the new code.
type TestProof struct {
	Name       string `json:"name"`
	FailsOnOld bool   `json:"fails_on_old"`
	PassesNew  bool   `json:"passes_on_new"`
}

func (p TestProof) Proven() bool { return p.FailsOnOld && p.PassesNew }

// CheckOutcome is the deterministic half of an attempt's verdict.
type CheckOutcome struct {
	Results  []CheckResult `json:"results"`
	Failures []Failure
	Blocked  []string
	Old      []string // failed before the task too, so not counted
	Proven   []string
	Unproven []string // passes on the old code too: tests nothing
}

// JudgeChecks subtracts old failures and applies the proven-test rule.
func JudgeChecks(results []CheckResult, base Baseline, proofs []TestProof) CheckOutcome {
	o := CheckOutcome{Results: results}
	for _, r := range results {
		switch r.Status {
		case CheckBlocked:
			o.Blocked = append(o.Blocked, r.Name)
		case CheckFail:
			if base.Failed[r.Name] {
				o.Old = append(o.Old, r.Name)
				continue
			}
			o.Failures = append(o.Failures, Failure{Source: r.Name, What: headline(r.Output), Lines: TailLines(r.Output, 30)})
		}
	}
	for _, p := range proofs {
		switch {
		case p.Proven():
			o.Proven = append(o.Proven, p.Name)
		case !p.PassesNew:
			o.Failures = append(o.Failures, Failure{Source: "test " + p.Name, What: "the builder's own new test fails on its change"})
		default:
			o.Unproven = append(o.Unproven, p.Name)
		}
	}
	return o
}

// headline is the first line a check printed, which is usually why it failed.
func headline(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 160 {
				l = l[:160] + "…"
			}
			return l
		}
	}
	return "check failed"
}
