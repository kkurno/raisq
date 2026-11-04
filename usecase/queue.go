package usecase

import (
	"fmt"
	"sort"

	"raisq/domain"
)

// Next is the first queued task, in the order added, whose After tasks are
// all done. A whole waits for its parts this way.
func Next(d Deps) (string, error) {
	names, err := d.Store.Tasks()
	if err != nil {
		return "", err
	}
	type q struct{ name, added string }
	var queue []q
	states := map[string]domain.TaskState{}
	for _, n := range names {
		st, err := d.Store.LoadState(n)
		if err != nil {
			continue
		}
		states[n] = st
		if st.State == domain.Queued {
			queue = append(queue, q{n, st.Added})
		}
	}
	sort.SliceStable(queue, func(i, j int) bool { return queue[i].added < queue[j].added })
	waiting := 0
	for _, t := range queue {
		ready := true
		for _, a := range states[t.name].After {
			if states[a].State != domain.Done {
				ready = false
			}
		}
		if ready {
			return t.name, nil
		}
		waiting++
	}
	if waiting > 0 {
		return "", fmt.Errorf("%d queued task(s) wait for parts that aren't done; see raisq status", waiting)
	}
	return "", ErrEmpty
}

var ErrEmpty = fmt.Errorf("nothing queued: raisq task \"...\" adds one")

// shareBase gives every part of a split task, and its whole, the same
// "before the task": the commit the first part started from.
func shareBase(d Deps, task string, st domain.TaskState) {
	whole := st.PartOf
	if whole == "" && st.CheckOnly {
		whole = task
	}
	if whole == "" {
		return
	}
	names, _ := d.Store.Tasks()
	for _, n := range names {
		o, err := d.Store.LoadState(n)
		if err != nil || n == task || o.Base != "" || (n != whole && o.PartOf != whole) {
			continue
		}
		o.Base = st.Base
		_ = d.Store.SaveState(n, o)
	}
}
