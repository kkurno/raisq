package usecase

import (
	"fmt"
	"path/filepath"
	"strings"

	"raisq/domain"
)

// ReviewPack is what the person reviewing reads, in the order they need it:
// what's unsure first, everything that passed last.
func ReviewPack(d Deps, task, run string, rec RunRecord, last AttemptRecord) string {
	var b strings.Builder
	spec, _ := d.Store.LoadSpec(task)
	fmt.Fprintf(&b, "# %s\n\n", firstNonEmpty(spec.Title, task))
	state := "ready for review"
	if rec.Parked != "" {
		state = fmt.Sprintf("parked: %s: %s", rec.Parked, rec.Note)
	}
	fmt.Fprintf(&b, "**%s** · %d attempt(s) · branch `%s` · facts@%s · spec@%s\n\n", state, rec.Attempts, rec.Branch, rec.Facts, rec.Spec)
	if d.Manual {
		b.WriteString("Built by a person (manual mode).\n\n")
	}
	for _, n := range d.Notices {
		b.WriteString("> ⚠ " + n + "\n")
	}
	if len(d.Notices) > 0 {
		b.WriteString("\n")
	}

	group := func(v domain.Verdict) []domain.Judgement {
		var out []domain.Judgement
		for _, j := range last.Judgements {
			if j.Verdict == v {
				out = append(out, j)
			}
		}
		return out
	}

	if us := group(domain.Unsure); len(us) > 0 {
		b.WriteString("## ❓ Unsure, read these first\n\n")
		for _, j := range us {
			fmt.Fprintf(&b, "- %s\n  - %s", quoteOr(j), j.Why)
			if j.FactID != "" {
				fmt.Fprintf(&b, " · `%s` (%s)", j.FactID, j.FactTag)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	noChange := last.Handoff != nil && len(last.Judgements) == 0 && len(last.Checks.Failures) == 0
	if fs := group(domain.Fail); len(fs) > 0 || len(last.Checks.Failures) > 0 || noChange {
		b.WriteString("## ✗ Failed\n\n")
		if noChange {
			for _, f := range last.Handoff.Failures {
				fmt.Fprintf(&b, "- %s: %s\n", f.Source, f.What)
			}
		}
		for _, f := range last.Checks.Failures {
			fmt.Fprintf(&b, "- check **%s**: %s\n", f.Source, f.What)
		}
		for _, j := range fs {
			fmt.Fprintf(&b, "- %s\n  - %s\n", quoteOr(j), j.Why)
		}
		b.WriteString("\n")
	}
	if cs := readChoices(filepath.Join(d.Store.RunDir(task, run), fmt.Sprintf("attempt-%d", last.N), "out")); len(cs) > 0 {
		b.WriteString("## 📝 Choices the spec didn't cover\n\n")
		for _, c := range cs {
			fmt.Fprintf(&b, "- %s", c.What)
			if c.Why != "" {
				fmt.Fprintf(&b, " (%s)", c.Why)
			}
			b.WriteString("\n")
		}
		b.WriteString("\nIf one of these matters, add it to the spec.\n\n")
	}
	if len(last.Checks.Proven)+len(last.Checks.Unproven) > 0 {
		b.WriteString("## Tests the builder added\n\n")
		for _, t := range last.Checks.Proven {
			fmt.Fprintf(&b, "- ✓ `%s`: fails without the change, passes with it\n", t)
		}
		for _, t := range last.Checks.Unproven {
			fmt.Fprintf(&b, "- ⚠ `%s`: passes on the old code too, so it tests nothing\n", t)
		}
		b.WriteString("\n")
	}
	if len(last.Checks.Old) > 0 {
		fmt.Fprintf(&b, "## Already failing before the task\n\n%s\n\n", strings.Join(last.Checks.Old, ", "))
	}
	if nas := group(domain.NotJudged); len(nas) > 0 {
		b.WriteString("## Not applicable\n\n")
		for _, j := range nas {
			fmt.Fprintf(&b, "- %s: %s\n", j.Subject, j.Why)
		}
		b.WriteString("\n")
	}
	if rec.Attempts > 1 {
		b.WriteString("## What earlier attempts got wrong\n\n")
		for n := 1; n < rec.Attempts; n++ {
			var ar AttemptRecord
			if d.Store.LoadRunFile(task, run, fmt.Sprintf("attempt-%d/attempt.json", n), &ar) == nil && ar.Handoff != nil {
				for _, f := range ar.Handoff.Failures {
					fmt.Fprintf(&b, "- attempt %d · %s: %s\n", n, f.Source, f.What)
				}
			}
		}
		b.WriteString("\n")
	}
	if ps := group(domain.Pass); len(ps) > 0 {
		b.WriteString("## ✓ Passed\n\n")
		for _, j := range ps {
			fmt.Fprintf(&b, "- %s · %s\n", quoteOr(j), j.Why)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Evidence: `%s`\n\n", d.Store.RunDir(task, run))
	if rec.Parked == "" {
		fmt.Fprintf(&b, "Decide: `raisq review %s --done` · `--send-back \"why\"` · or change the spec in `tasks/%s/spec.yaml` and send it back\n", task, task)
	} else {
		fmt.Fprintf(&b, "Decide: `raisq review %s --answer \"...\"` to requeue it, or `--send-back \"why\"`\n", task)
	}
	return b.String()
}

func quoteOr(j domain.Judgement) string {
	if j.Kind == "claim" {
		return fmt.Sprintf("%q", j.Subject)
	}
	return j.Subject
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
