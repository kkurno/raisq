package builtin

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"raisq/slot"
)

// PageText is the web preset's extractor: the text of the site's pages, as a
// user reads it, for the pages whose text the task changed (4.7). It crawls
// the running site from inside the session, so it needs `site`.
type PageText struct {
	Port  int
	Start []string // paths to start from; "/" by default
	Max   int      // pages per crawl
}

func (PageText) Name() string    { return "page-text" }
func (PageText) Needs() []string { return []string{"site"} }

// Extract renders the site as the task left it, and as it was at the base
// commit, and keeps the pages whose text changed, whole, with the changed
// lines marked +. Text that was there before the task isn't this task's to
// answer for, but it's shown around the change so the critic can read it.
func (p PageText) Extract(ctx context.Context, r slot.ExtractRequest) (string, error) {
	if r.Session == nil {
		return "", nil
	}
	after, err := p.crawl(ctx, r.Session)
	if err != nil || len(after) == 0 {
		return "", err
	}
	before := r.Before
	if before == nil {
		before = map[string]string{}
	}
	var paths []string
	for path := range after {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, path := range paths {
		now := after[path]
		was, existed := before[path]
		if existed && was == now {
			continue
		}
		fmt.Fprintf(&b, "## page %s\n", path)
		old := map[string]bool{}
		for _, l := range strings.Split(was, "\n") {
			old[l] = true
		}
		for _, l := range strings.Split(now, "\n") {
			if l == "" {
				continue
			}
			if old[l] {
				b.WriteString("  " + l + "\n")
			} else {
				b.WriteString("+ " + l + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// Snapshot is the text of every page, for the base commit's render.
func (p PageText) Snapshot(ctx context.Context, s slot.Session) (map[string]string, error) {
	return p.crawl(ctx, s)
}

func (p PageText) crawl(ctx context.Context, s slot.Session) (map[string]string, error) {
	port := p.Port
	if port == 0 {
		port = 3000
	}
	max := p.Max
	if max == 0 {
		max = 40
	}
	base := fmt.Sprintf("http://localhost:%d", port)
	queue := p.Start
	if len(queue) == 0 {
		queue = []string{"/"}
	}
	seen := map[string]bool{}
	pages := map[string]string{}
	for len(queue) > 0 && len(pages) < max {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true
		r, err := s.Exec(ctx, "curl -fsSL --max-time 20 "+shellQuote(base+path)+" || wget -q -O - "+shellQuote(base+path), nil)
		if err != nil {
			return nil, err
		}
		if r.Exit != 0 {
			continue
		}
		pages[path] = Text(r.Output)
		for _, link := range links(r.Output) {
			if !seen[link] {
				queue = append(queue, link)
			}
		}
	}
	return pages, nil
}

var (
	reDrop  = dropTags("script", "style", "noscript", "svg", "template")
	reBreak = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6]|/tr|/section|/article|/header|/footer|/nav|/main|/form|/label|/button|/option|/td|/th|/dt|/dd)\b[^>]*>`)
	reAttr  = regexp.MustCompile(`(?i)<(?:input|img|textarea|select)\b[^>]*?\b(?:placeholder|alt|value|aria-label)\s*=\s*"([^"]*)"[^>]*>`)
	reTag   = regexp.MustCompile(`<[^>]+>`)
	reSpace = regexp.MustCompile(`[ \t\r\f\v]+`)
	reHref  = regexp.MustCompile(`(?i)\bhref\s*=\s*"(/[^"#?]*)`)
)

// Text is what a user reads on a page: visible text, and the words inside
// form fields and images, one block per line.
func Text(page string) string {
	s := page
	for _, re := range reDrop {
		s = re.ReplaceAllString(s, "")
	}
	s = reAttr.ReplaceAllString(s, "\n[$1]\n")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(reSpace.ReplaceAllString(l, " "))
		if l != "" && l != "[]" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// dropTags matches each element whole. Go's regexp has no backreferences, so
// it's one pattern per tag.
func dropTags(tags ...string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, t := range tags {
		out = append(out, regexp.MustCompile(`(?is)<`+t+`\b.*?</\s*`+t+`\s*>`))
	}
	return out
}

func links(page string) []string {
	var out []string
	for _, m := range reHref.FindAllStringSubmatch(page, -1) {
		p := m[1]
		if strings.HasPrefix(p, "//") || strings.Contains(p, ".") && !strings.HasSuffix(p, ".html") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
