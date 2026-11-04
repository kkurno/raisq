package usecase

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
)

// RetireAfter is how many times a person may overrule a lesson before
// raisq learn proposes retiring it.
const RetireAfter = 3

// Lesson is what raisq learn proposes from one mistake.
type Lesson struct {
	Task string
	What string
	// Check is a command that exits 0 when the mistake is absent. Empty means
	// the lesson needs judgment and becomes a rubric.
	Check  string
	Rubric domain.Rubric
	Proven bool
	Why    string
}

// ProposeLesson drafts a lesson. A check is tried first, because a command
// beats an AI's opinion when a command will do; it counts only if it fails
// on the commit that had the mistake and passes on the commit that fixed it.
func ProposeLesson(ctx context.Context, d Deps, task, what, check, bad, good string) (Lesson, error) {
	l := Lesson{Task: task, What: what, Check: check}
	if check != "" {
		if bad == "" || good == "" {
			return l, fmt.Errorf("a check from a lesson is proven on two commits: --bad <commit with the mistake> --good <commit without it>")
		}
		failsBad, err := runAt(ctx, d, task, bad, check)
		if err != nil {
			return l, err
		}
		failsGood, err := runAt(ctx, d, task, good, check)
		if err != nil {
			return l, err
		}
		switch {
		case !failsBad:
			l.Why = "it passes on the commit that had the mistake, so it wouldn't have caught it"
		case failsGood:
			l.Why = "it fails on the commit that fixed the mistake too, so it would fail everything"
		default:
			l.Proven = true
			l.Why = "fails on the mistake, passes on the fix"
		}
		return l, nil
	}
	var r domain.Rubric
	prompt := `A mistake got past review. Write one standing question a reviewer could ask of any future output to catch this kind of mistake. It must say how to check, never what's true: facts belong elsewhere.

Mistake: ` + what + `

Answer with JSON only: {"ask":"...","fail_if":"yes|no"}`
	if err := askJSON(ctx, d, prompt, &r); err != nil {
		return l, err
	}
	if r.Ask == "" || (r.FailIf != domain.Yes && r.FailIf != domain.No) {
		return l, fmt.Errorf("the model didn't propose a usable question")
	}
	r.ID, r.Lesson = "lesson/"+slug(what), "task/"+task
	l.Rubric = r
	return l, nil
}

// runAt runs a command on a commit, inside isolation, and reports whether it
// failed.
func runAt(ctx context.Context, d Deps, task, rev, cmd string) (bool, error) {
	dir, err := d.Checkout.Prepare(ctx, "learn-"+task, "raisq-learn", rev)
	if err != nil {
		return false, err
	}
	defer d.Checkout.Remove(dir)
	out, err := os.MkdirTemp("", "raisq-learn-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(out)
	s, err := d.Isolation.Start(ctx, slot.StartRequest{Checkout: dir, OutDir: out, Label: "learn"})
	if err != nil {
		return false, err
	}
	defer s.Stop(ctx)
	if err := install(ctx, s); err != nil {
		return false, err
	}
	r, err := s.Exec(ctx, cmd, nil)
	if err != nil {
		return false, err
	}
	if r.Exit == 127 {
		return false, fmt.Errorf("the check's tool isn't there: %s", domain.TailLines(r.Output, 3))
	}
	return r.Exit != 0, nil
}

// KeepLesson writes an approved lesson where checks and rubrics live.
func KeepLesson(d Deps, l Lesson) (string, error) {
	if l.Check != "" {
		if !l.Proven {
			return "", fmt.Errorf("not kept: %s", l.Why)
		}
		name := "lesson-" + slug(l.What)
		body := fmt.Sprintf("name: %s\nslot: checks\npriority: 50\nrun: %s\nlesson: task/%s\n# %s\n", name, yamlString(l.Check), l.Task, strings.ReplaceAll(l.What, "\n", " "))
		p := filepath.Join(d.Store.Root(), "plugins", name+".yaml")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		return p, os.WriteFile(p, []byte(body), 0o644)
	}
	return filepath.Join(d.Store.Root(), "rubrics", "lessons.yaml"), d.Store.SaveRubric(l.Rubric, "lessons")
}

func yamlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Overrule records that a person marked a lesson's fail wrong at review.
// Past RetireAfter, it says so: retiring is still the person's call.
func Overrule(d Deps, rubricID string) (int, error) {
	rs, err := d.Store.LoadRubrics()
	if err != nil {
		return 0, err
	}
	for _, r := range rs {
		if r.ID != rubricID {
			continue
		}
		if r.Lesson == "" {
			return 0, fmt.Errorf("%s isn't a lesson: change the preset's rubric file instead", rubricID)
		}
		r.Overruled++
		return r.Overruled, d.Store.SaveRubric(r, "lessons")
	}
	return 0, fmt.Errorf("no rubric %s", rubricID)
}

// Retire stops a lesson running and keeps it on file, so it isn't re-learned.
func Retire(d Deps, rubricID string) error {
	rs, err := d.Store.LoadRubrics()
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.ID == rubricID {
			r.Retired = true
			return d.Store.SaveRubric(r, "lessons")
		}
	}
	return fmt.Errorf("no rubric %s", rubricID)
}

// ToRetire is every lesson overruled often enough to propose retiring.
func ToRetire(d Deps) ([]domain.Rubric, error) {
	rs, err := d.Store.LoadRubrics()
	var out []domain.Rubric
	for _, r := range rs {
		if r.Lesson != "" && !r.Retired && r.Overruled >= RetireAfter {
			out = append(out, r)
		}
	}
	return out, err
}

// SpotCheck picks a share of done tasks for a person to re-read.
func SpotCheck(d Deps, share float64, rng *rand.Rand) ([]string, error) {
	names, err := d.Store.Tasks()
	if err != nil {
		return nil, err
	}
	var done []string
	for _, n := range names {
		if st, err := d.Store.LoadState(n); err == nil && st.State == domain.Done && st.PartOf == "" {
			done = append(done, n)
		}
	}
	rng.Shuffle(len(done), func(i, j int) { done[i], done[j] = done[j], done[i] })
	n := int(float64(len(done))*share + 0.999)
	if n > len(done) {
		n = len(done)
	}
	return done[:n], nil
}
