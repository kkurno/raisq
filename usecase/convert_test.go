package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot/fake"
)

const specKit = `# Feature: Cake orders

## Requirements
- FR-001: Customers MUST pick a delivery date.
- FR-002: Orders MUST need 2 days' notice.
- Delivery is offered within 5 km.
`

func TestConvertKeepsEveryLine(t *testing.T) {
	m := &fake.Model{Fn: func(string) string {
		return `{"title":"Cake orders","items":[
		 {"kind":"criterion","id":"c1","text":"Customers pick a delivery date","quote":"FR-001: Customers MUST pick a delivery date."},
		 {"kind":"criterion","id":"c2","text":"Orders need 2 days' notice","quote":"FR-002: Orders MUST need 2 days' notice."},
		 {"kind":"fact","id":"radius","topic":"orders","text":"Delivery is offered within 5 km","quote":"Delivery is offered within 5 km."}],
		 "skipped":["Feature: Cake orders","Requirements"]}`
	}}
	spec, facts, err := Convert(context.Background(), Deps{Model: m}, "cake-orders", "spec-kit", specKit)
	must(t, err)
	if len(spec.Criteria) != 2 || !spec.Outside {
		t.Fatalf("%+v", spec)
	}
	if len(facts) != 1 || facts[0].ID != "orders/radius" || facts[0].Tag != domain.Inferred {
		t.Fatalf("a converted fact is inferred until a person confirms it: %+v", facts)
	}
}

func TestConvertStopsOnADroppedLine(t *testing.T) {
	m := &fake.Model{Fn: func(string) string {
		return `{"title":"Cake orders","items":[
		 {"kind":"criterion","id":"c1","text":"Customers pick a delivery date","quote":"FR-001: Customers MUST pick a delivery date."}],
		 "skipped":["Feature: Cake orders","Requirements"]}`
	}}
	_, _, err := Convert(context.Background(), Deps{Model: m}, "cake-orders", "spec-kit", specKit)
	if err == nil || !strings.Contains(err.Error(), "2 days' notice") {
		t.Fatalf("a dropped line must stop the conversion: %v", err)
	}
}

func TestConvertStopsOnAnInventedQuote(t *testing.T) {
	m := &fake.Model{Fn: func(string) string {
		return `{"title":"x","items":[
		 {"kind":"criterion","id":"c1","text":"x","quote":"FR-001: Customers MUST pick a delivery date."},
		 {"kind":"criterion","id":"c2","text":"x","quote":"FR-002: Orders MUST need 2 days' notice."},
		 {"kind":"criterion","id":"c3","text":"x","quote":"Delivery is offered within 5 km."},
		 {"kind":"criterion","id":"c4","text":"free cake","quote":"Every order gets a free cake."}],
		 "skipped":["Feature: Cake orders","Requirements"]}`
	}}
	if _, _, err := Convert(context.Background(), Deps{Model: m}, "x", "spec-kit", specKit); err == nil {
		t.Fatal("a quote that isn't in the file must stop the conversion")
	}
}

func TestDraftPluginRetriesOnFailures(t *testing.T) {
	m := &fake.Model{Fn: func(p string) string {
		if strings.Contains(p, "failed the plugin test") {
			return "```yaml\nname: mytool\nslot: checks\nrun: mytool check\n```"
		}
		return "name: mytool\nslot: checks\nrun: <command>\n"
	}}
	test := func(ctx context.Context, body string) []string {
		if strings.Contains(body, "<command>") {
			return []string{"has template parts left to fill"}
		}
		return nil
	}
	dr, err := DraftPlugin(context.Background(), Deps{Model: m}, "mytool", "checks", "", "help", "", test, 3)
	must(t, err)
	if dr.Tries != 2 || !strings.Contains(dr.Body, "run: mytool check") || strings.Contains(dr.Body, "```") {
		t.Fatalf("%+v", dr)
	}
}
