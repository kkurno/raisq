package command

import (
	"regexp"
	"sync"
)

var (
	reMu    sync.Mutex
	reCache = map[string]*regexp.Regexp{}
)

func matchRegexp(pattern, s string) (bool, error) {
	reMu.Lock()
	re, ok := reCache[pattern]
	reMu.Unlock()
	if !ok {
		var err error
		if re, err = regexp.Compile("^(?:" + pattern + ")$"); err != nil {
			return false, err
		}
		reMu.Lock()
		reCache[pattern] = re
		reMu.Unlock()
	}
	return re.MatchString(s), nil
}
