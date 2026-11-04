package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
)

func TestText(t *testing.T) {
	page := `<html><head><style>.x{}</style><script>var a=1</script></head><body>
<nav><a href="/menu">Menu</a></nav>
<h1>Sweet Crumbs &amp; Co</h1><p>Call   02-123-4567</p>
<form><input placeholder="Your phone"><button>Order</button></form></body></html>`
	got := Text(page)
	for _, want := range []string{"Sweet Crumbs & Co", "Call 02-123-4567", "[Your phone]", "Order", "Menu"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "var a") || strings.Contains(got, ".x{}") {
		t.Error("scripts and styles aren't what a user reads")
	}
	if l := links(page); len(l) != 1 || l[0] != "/menu" {
		t.Errorf("links: %v", l)
	}
}

type pages map[string]string

func (p pages) Paths() slot.Paths { return slot.Paths{} }
func (p pages) Exec(ctx context.Context, cmd string, env map[string]string) (slot.ExecResult, error) {
	for path, body := range p {
		if strings.Contains(cmd, "'http://localhost:3000"+path+"'") {
			return slot.ExecResult{Output: body}, nil
		}
	}
	return slot.ExecResult{Exit: 22}, nil
}
func (p pages) Background(context.Context, string, map[string]string) error { return nil }
func (p pages) URL(int) (string, error)                                     { return "", nil }
func (p pages) Stop(context.Context) error                                  { return nil }

func TestPageTextReturnsOnlyChangedPages(t *testing.T) {
	site := pages{
		"/":      `<h1>Home</h1><a href="/order">Order</a><a href="/about">About</a>`,
		"/order": `<h1>Order</h1><p>Delivery within 5 km</p>`,
		"/about": `<h1>About</h1><p>Since 2010</p>`,
	}
	pt := PageText{}
	before, err := pt.Snapshot(context.Background(), site)
	if err != nil {
		t.Fatal(err)
	}
	site["/order"] = `<h1>Order</h1><p>Delivery within 10 km</p>`
	out, err := pt.Extract(context.Background(), slot.ExtractRequest{Session: site, Before: before})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "+ Delivery within 10 km") || !strings.Contains(out, "  Order") {
		t.Fatalf("changed lines are marked, the rest of the page shown:\n%s", out)
	}
	if strings.Contains(out, "Since 2010") {
		t.Fatal("an unchanged page isn't this task's to answer for")
	}
}

func TestDiffDropsLockfiles(t *testing.T) {
	d := "diff --git a/src/x.js b/src/x.js\n+hi\ndiff --git a/package-lock.json b/package-lock.json\n+huge\n"
	got := dropLockfiles(d)
	if strings.Contains(got, "huge") || !strings.Contains(got, "+hi") {
		t.Fatal(got)
	}
}

func TestContacts(t *testing.T) {
	site := pages{"/": `<p>Call 02-123-4567 or 02 999 8888</p><p>hi@shop.test</p>`}
	facts := domain.Facts{Items: []domain.Fact{
		{ID: "b/phone", Text: "02-123-4567", Tag: domain.Confirmed},
		{ID: "b/email", Text: "hi@shop.test", Tag: domain.Confirmed},
	}}
	c := Contacts{Facts: func() (domain.Facts, error) { return facts, nil }}
	r := c.Run(context.Background(), site, nil)
	if r.Status != domain.CheckFail || !strings.Contains(r.Output, "02 999 8888") || strings.Contains(r.Output, "123-4567") {
		t.Fatalf("%s: %s", r.Status, r.Output)
	}
}
