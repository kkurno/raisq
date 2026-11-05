package proc

import "strings"

// UsageLimited spots an agent CLI's vendor saying the account is out of
// quota. Nothing else in an agent's output is ever read for meaning.
func UsageLimited(out string) bool {
	if len(out) > 2000 {
		out = out[len(out)-2000:]
	}
	l := strings.ToLower(out)
	for _, s := range []string{"usage limit", "rate limit", "quota", "429", "credit balance is too low"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}
