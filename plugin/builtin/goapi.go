package builtin

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"raisq/domain"
	"raisq/slot"
)

// GoAPI reports when the exported Go API changed. It compares `go doc` on the
// task's result with the same listing at the base commit, which the baseline
// records once per base commit.
type GoAPI struct {
	CheckName string
	Store     slot.Store
	Base      func() string
	// Allowed is true when the task spec says the API may change: the change
	// is then reported, not failed.
	Allowed bool
}

func (g GoAPI) Name() string    { return firstOr(g.CheckName, "go-api") }
func (g GoAPI) Priority() int   { return 25 }
func (g GoAPI) Needs() []string { return nil }
func (g GoAPI) Locks() []string { return nil }
func (g GoAPI) Tests() []string { return nil }
func (g GoAPI) RunOne(context.Context, slot.Session, map[string]string, string) (domain.CheckResult, bool) {
	return domain.CheckResult{}, false
}

const apiScript = `go list ./... 2>/dev/null | while read p; do go doc -short "$p" 2>/dev/null | sed "s|^|$p: |"; done`

func listAPI(ctx context.Context, s slot.Session) (map[string]string, error) {
	r, err := s.Exec(ctx, apiScript, nil)
	if err != nil {
		return nil, err
	}
	if r.Exit == 127 {
		return nil, fmt.Errorf("go isn't in the container")
	}
	m := map[string]string{}
	for _, l := range strings.Split(r.Output, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			m[l] = ""
		}
	}
	return m, nil
}

// Snapshot is the API at the base commit.
func (g GoAPI) Snapshot(ctx context.Context, s slot.Session) (map[string]string, error) {
	return listAPI(ctx, s)
}

func (g GoAPI) Run(ctx context.Context, s slot.Session, env map[string]string) domain.CheckResult {
	res := domain.CheckResult{Name: g.Name()}
	now, err := listAPI(ctx, s)
	if err != nil {
		res.Status, res.Output = domain.CheckBlocked, err.Error()
		return res
	}
	was, found, err := g.Store.Snapshot(g.Base(), g.Name())
	if err != nil || !found {
		// The baseline run is measuring the base itself: nothing to compare.
		res.Status, res.Output = domain.CheckPass, "no base API recorded yet"
		return res
	}
	var gone, added []string
	for k := range was {
		if _, ok := now[k]; !ok {
			gone = append(gone, "- "+k)
		}
	}
	for k := range now {
		if _, ok := was[k]; !ok {
			added = append(added, "+ "+k)
		}
	}
	sort.Strings(gone)
	sort.Strings(added)
	if len(gone)+len(added) == 0 {
		res.Status, res.Output = domain.CheckPass, "public API unchanged"
		return res
	}
	res.Output = strings.Join(append(gone, added...), "\n")
	// Adding to the API is compatible; removing or changing a signature isn't.
	if len(gone) == 0 || g.Allowed {
		res.Status = domain.CheckPass
		res.Output = "public API changed, and that's allowed:\n" + res.Output
		return res
	}
	res.Status = domain.CheckFail
	res.Output = "the public API lost or changed these, and the spec doesn't say it may:\n" + res.Output
	return res
}
