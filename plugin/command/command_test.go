package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/slot/fake"
)

func session(t *testing.T) slot.Session {
	t.Helper()
	s, err := fake.Isolation{}.Start(context.Background(), slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadAndChecks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "unit.yaml", "name: unit\nslot: checks\nrun: exit 1\n")
	write(t, dir, "lh.yaml", `name: lh
slot: checks
run: 'echo "report {\"categories\":{\"performance\":{\"score\":0.93}}}"'
result: {from: json, path: categories.performance.score, scale: 100}
pass_at: 90
median: 3
`)
	write(t, dir, "missing.yaml", "name: missing\nslot: checks\nrun: no-such-tool-xyz\n")
	fs, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.CheckResult{}
	for _, f := range fs {
		got[f.Name] = Check{F: f}.Run(context.Background(), session(t), nil)
	}
	if got["unit"].Status != domain.CheckFail {
		t.Error("exit 1 is a fail")
	}
	if got["lh"].Status != domain.CheckPass || *got["lh"].Score != 93 {
		t.Errorf("score: %+v", got["lh"])
	}
	if got["missing"].Status != domain.CheckBlocked {
		t.Error("a missing tool blocks, never passes")
	}
}

func TestProviderInjectsEnvForChecksOnly(t *testing.T) {
	proj := t.TempDir()
	write(t, proj, ".env.test", "# test\nDB_URL=postgres://test\nexport KEY='abc'\n")
	p := Provider{F: File{Name: "env.test", Slot: "provides", From: "host:.env.test"}, Project: proj}
	if _, ok := p.Mount(); ok {
		t.Fatal("a checks provider is never mounted for the builder")
	}
	env, err := p.Start(context.Background(), session(t))
	if err != nil {
		t.Fatal(err)
	}
	if env["DB_URL"] != "postgres://test" || env["KEY"] != "abc" {
		t.Fatalf("env %v", env)
	}
	b := Provider{F: File{Name: "ds", Slot: "provides", For: "builder", From: "host:../ds:ro"}, Project: proj}
	m, ok := b.Mount()
	if !ok || !m.ReadOnly {
		t.Fatal("a builder provider mounts read-only")
	}
}

func TestSourceMatches(t *testing.T) {
	s := Source{F: File{Name: "jira", Matches: "[A-Z]+-[0-9]+"}}
	if !s.Matches("SHOP-123") || s.Matches("shop-123") || s.Matches("SHOP-123x") {
		t.Fatal("matches must be anchored")
	}
}

func TestModel(t *testing.T) {
	m := Model{F: File{Name: "echo", Run: "cat"}}
	out, err := m.Ask(context.Background(), `{"ok":true}`)
	if err != nil || out != `{"ok":true}` {
		t.Fatalf("%q %v", out, err)
	}
}

// A critic out of quota stops the run, as the builder does, instead of
// parking the task as blocked.
func TestModelUsageLimitIsAStop(t *testing.T) {
	m := Model{F: File{Name: "critic", Run: "echo 'Claude usage limit reached' >&2; exit 1"}}
	if _, err := m.Ask(context.Background(), "x"); !errors.Is(err, slot.ErrUsageLimit) {
		t.Fatalf("want a usage limit, got %v", err)
	}
	m = Model{F: File{Name: "critic", Run: "echo 'bad flag' >&2; exit 2"}}
	if _, err := m.Ask(context.Background(), "x"); errors.Is(err, slot.ErrUsageLimit) {
		t.Fatal("any other failure isn't a usage limit")
	}
}

// A plugin file extractor's needs start their providers, as a check's do.
func TestExtractorNeeds(t *testing.T) {
	var e slot.Extractor = Extractor{F: File{Name: "pages", Needs: []string{"site"}}}
	n, ok := e.(slot.Needer)
	if !ok || len(n.Needs()) != 1 || n.Needs()[0] != "site" {
		t.Fatal("an extractor's needs must reach the run")
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestContract(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		ok         bool
	}{
		{"check", "name: unit\nslot: checks\nrun: true\n", true},
		{"unfilled", "name: x\nslot: checks\nrun: <command>\n", false},
		{"builder without prompt", "name: b\nslot: builder\nvendor: v\nrun: agent go\n", false},
		{"builder", "name: b\nslot: builder\nvendor: v\nrun: agent < {prompt}\n", true},
		{"writable builder mount", "name: p\nslot: provides\nfor: builder\nfrom: host:../ds\n", false},
		{"bad regexp", "name: s\nslot: spec-source\nrun: x {id}\nmatches: \"[\"\n", false},
		{"unknown slot", "name: s\nslot: nope\n", false},
	}
	for _, c := range cases {
		bad := Contract(context.Background(), c.body, dir)
		if (len(bad) == 0) != c.ok {
			t.Errorf("%s: want ok=%v, got %v", c.name, c.ok, bad)
		}
	}
}

// A check in one package of a monorepo runs there, and its globs and single
// test file are relative to that package, so RaisQ can still lock its config
// and prove a test the builder adds.
func TestCheckInADir(t *testing.T) {
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, "apps", "api", "test"), 0o755)
	write(t, filepath.Join(proj, "apps", "api"), "marker", "api")
	write(t, filepath.Join(proj, "apps", "api", "test"), "a.spec.ts", "")
	s, _ := fake.Isolation{}.Start(context.Background(), slot.StartRequest{Checkout: proj, OutDir: t.TempDir()})
	c := Check{F: File{Name: "api-test", Slot: "checks", Dir: "apps/api/",
		Run: "test -f marker", RunOne: "test -f {file}", Tests: []string{"**/*.spec.ts"}, Locks: []string{"jest.config.*"}}}
	if r := c.Run(context.Background(), s, nil); r.Status != domain.CheckPass {
		t.Fatalf("the check should run in its dir: %+v", r)
	}
	if got := c.Tests(); len(got) != 1 || got[0] != "apps/api/**/*.spec.ts" {
		t.Fatalf("tests should be relative to the project: %v", got)
	}
	if got := c.Locks(); len(got) != 1 || got[0] != "apps/api/jest.config.*" {
		t.Fatalf("locks should be relative to the project: %v", got)
	}
	if r, ok := c.RunOne(context.Background(), s, nil, "apps/api/test/a.spec.ts"); !ok || r.Status != domain.CheckPass {
		t.Fatalf("one test file runs by its path in the dir: %+v", r)
	}
}

// A service the checks need, where the isolation can't start one, blocks with
// what to do; it never quietly runs the checks without it.
func TestServiceNeedsAContainerTool(t *testing.T) {
	p := Provider{F: File{Name: "postgres", Slot: "provides", Image: "postgres:16"}}
	_, err := p.Start(context.Background(), session(t))
	if err == nil || !strings.Contains(err.Error(), "docker or podman") {
		t.Fatalf("want an error saying what to use, got %v", err)
	}
}

// A check's own env sets where things are; what a provider injects wins, so a
// committed file can't override a secret.
func TestCheckEnv(t *testing.T) {
	c := Check{F: File{Name: "e", Slot: "checks", Run: `[ "$DB" = "postgres" ] && [ "$TOKEN" = "secret" ]`,
		Env: map[string]string{"DB": "postgres", "TOKEN": "from-file"}}}
	if r := c.Run(context.Background(), session(t), map[string]string{"TOKEN": "secret"}); r.Status != domain.CheckPass {
		t.Fatalf("want the check's env plus the provider's, provider winning: %+v", r)
	}
}

// Checking a plugin's contract never runs a check's command on this machine:
// that's the project's code, which runs only in isolation.
func TestContractNeverRunsACheck(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran-on-host")
	body := "name: sneaky\nslot: checks\nrun: touch " + marker + "\n"
	if bad := Contract(context.Background(), body, dir); len(bad) != 0 {
		t.Fatalf("a well-formed check fills its contract: %v", bad)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the contract check ran the check's command on the host")
	}
}
