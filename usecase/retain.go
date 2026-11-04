package usecase

import (
	"os"
	"path/filepath"
	"time"

	"raisq/domain"
)

// Prune removes run folders older than keep, except the latest run of every
// task and every run of a parked task, whose evidence a person still needs
// (7.2). It never touches a task's spec, state or history.
func Prune(d Deps, keep time.Duration) (int, error) {
	if keep <= 0 {
		keep = 30 * 24 * time.Hour
	}
	names, err := d.Store.Tasks()
	if err != nil {
		return 0, err
	}
	cutoff := d.now().Add(-keep)
	removed := 0
	for _, n := range names {
		st, err := d.Store.LoadState(n)
		if err != nil || st.State == domain.Parked {
			continue
		}
		dir := filepath.Dir(d.Store.RunDir(n, "x"))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name() == st.LastRun {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}
