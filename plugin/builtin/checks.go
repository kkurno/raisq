package builtin

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"raisq/domain"
	"raisq/slot"
)

// Contacts fails when a page shows a phone number or an email that isn't in
// the facts. It's the most common mistake on a small site, so it's built in.
type Contacts struct {
	CheckName string
	Facts     func() (domain.Facts, error)
	Pages     PageText
}

func (c Contacts) Name() string    { return firstOr(c.CheckName, "contacts") }
func (c Contacts) Priority() int   { return 35 }
func (c Contacts) Needs() []string { return []string{"site"} }
func (c Contacts) Locks() []string { return nil }
func (c Contacts) Tests() []string { return nil }
func (c Contacts) RunOne(context.Context, slot.Session, map[string]string, string) (domain.CheckResult, bool) {
	return domain.CheckResult{}, false
}

var (
	rePhone = regexp.MustCompile(`(?:\+?\d[\d ()\-.]{6,}\d)`)
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
)

func (c Contacts) Run(ctx context.Context, s slot.Session, env map[string]string) domain.CheckResult {
	res := domain.CheckResult{Name: c.Name()}
	facts, err := c.Facts()
	if err != nil {
		res.Status, res.Output = domain.CheckBlocked, err.Error()
		return res
	}
	known := map[string]bool{}
	for _, f := range facts.Items {
		if f.Tag == domain.Gap {
			continue
		}
		for _, m := range rePhone.FindAllString(f.Text, -1) {
			known[digits(m)] = true
		}
		for _, m := range reEmail.FindAllString(f.Text, -1) {
			known[strings.ToLower(m)] = true
		}
	}
	pages, err := c.Pages.crawl(ctx, s)
	if err != nil {
		res.Status, res.Output = domain.CheckBlocked, err.Error()
		return res
	}
	var bad []string
	paths := sortedKeys(pages)
	for _, path := range paths {
		text := pages[path]
		for _, m := range rePhone.FindAllString(text, -1) {
			d := digits(m)
			if len(d) >= 8 && len(d) <= 15 && !known[d] {
				bad = append(bad, fmt.Sprintf("%s: phone %q isn't in the facts", path, strings.TrimSpace(m)))
			}
		}
		for _, m := range reEmail.FindAllString(text, -1) {
			if !known[strings.ToLower(m)] {
				bad = append(bad, fmt.Sprintf("%s: email %q isn't in the facts", path, m))
			}
		}
	}
	if len(bad) > 0 {
		res.Status, res.Output = domain.CheckFail, strings.Join(dedupe(bad), "\n")
		return res
	}
	res.Status, res.Output = domain.CheckPass, fmt.Sprintf("%d pages: every contact detail is in the facts", len(pages))
	return res
}

// Links fails on an internal link that doesn't resolve.
type Links struct {
	CheckName string
	Pages     PageText
}

func (l Links) Name() string    { return firstOr(l.CheckName, "links") }
func (l Links) Priority() int   { return 35 }
func (l Links) Needs() []string { return []string{"site"} }
func (l Links) Locks() []string { return nil }
func (l Links) Tests() []string { return nil }
func (l Links) RunOne(context.Context, slot.Session, map[string]string, string) (domain.CheckResult, bool) {
	return domain.CheckResult{}, false
}

func (l Links) Run(ctx context.Context, s slot.Session, env map[string]string) domain.CheckResult {
	res := domain.CheckResult{Name: l.Name()}
	port := l.Pages.Port
	if port == 0 {
		port = 3000
	}
	base := fmt.Sprintf("http://localhost:%d", port)
	seen := map[string]bool{}
	queue := []string{"/"}
	var broken []string
	for len(queue) > 0 && len(seen) < 200 {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true
		r, err := s.Exec(ctx, "curl -sSL --max-time 20 -o /tmp/raisq-page -w '%{http_code}' "+shellQuote(base+path)+" && echo && cat /tmp/raisq-page", nil)
		if err != nil {
			res.Status, res.Output = domain.CheckBlocked, err.Error()
			return res
		}
		code, body, _ := strings.Cut(r.Output, "\n")
		if r.Exit == 127 {
			res.Status, res.Output = domain.CheckBlocked, "curl isn't in the container"
			return res
		}
		if !strings.HasPrefix(strings.TrimSpace(code), "2") && !strings.HasPrefix(strings.TrimSpace(code), "3") {
			broken = append(broken, fmt.Sprintf("%s: %s", path, strings.TrimSpace(code)))
			continue
		}
		for _, link := range links(body) {
			if !seen[link] {
				queue = append(queue, link)
			}
		}
	}
	if len(broken) > 0 {
		res.Status, res.Output = domain.CheckFail, "broken internal links:\n"+strings.Join(broken, "\n")
		return res
	}
	res.Status, res.Output = domain.CheckPass, fmt.Sprintf("%d internal pages, none broken", len(seen))
	return res
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func firstOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
