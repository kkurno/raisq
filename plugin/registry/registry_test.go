package registry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkurno/raisq/plugin/command"
	"github.com/kkurno/raisq/slot"
)

// Every folder raisq skill can write to is kept from the builder, including
// a known agent's whose plugin file the project doesn't have.
func TestSkillFoldersArePrivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".raisq", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, "builder: cursor\ncritic: cursor-critic\n")
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{".raisq": false, ".agents/skills/raisq": false, ".claude/skills/raisq": false}
	for _, path := range p.Git.Private {
		if _, ok := want[path]; ok {
			want[path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("%s must be kept from the builder; private is %v", path, p.Git.Private)
		}
	}
	if f, ok := p.Agent("claude-code"); !ok || f.Skills != ".claude/skills" {
		t.Fatalf("a built-in agent is known without its file in the project: %+v", f)
	}
}

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	p := ConfigPath(filepath.Join(dir, ".raisq"))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newProject(t *testing.T, yaml string, plugins map[string]string) *Project {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".raisq", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, yaml)
	for name, body := range plugins {
		if err := os.WriteFile(filepath.Join(dir, ".raisq", "plugins", name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reopen(t *testing.T, p *Project) *Project {
	t.Helper()
	q, err := Open(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// Choosing a built-in agent brings its file in; a project's own file wins.
func TestSetBuilderBringsInTheBuiltInFile(t *testing.T) {
	p := newProject(t, "# mine\nbuilder: claude-code # who builds\ncritic: cursor-critic\n", map[string]string{
		"cursor-critic": "name: cursor-critic\nslot: model\nrun: x\n",
	})
	if _, err := p.Set("builder", "cursor", false); err != nil {
		t.Fatal(err)
	}
	q := reopen(t, p)
	if q.Config.Builder != "cursor" {
		t.Fatalf("builder %q", q.Config.Builder)
	}
	if _, ok := q.find("builder", "cursor"); !ok {
		t.Fatal("cursor's plugin file should be in the project")
	}
	data, _ := os.ReadFile(ConfigPath(p.Raisq))
	if !strings.Contains(string(data), "# mine") || !strings.Contains(string(data), "# who builds") {
		t.Fatalf("a person's comments must survive: %s", data)
	}

	mine := "name: claude-code\nslot: builder\nrun: my own command\n"
	os.WriteFile(filepath.Join(p.Raisq, "plugins", "claude-code.yaml"), []byte(mine), 0o644)
	q = reopen(t, q)
	if _, err := q.Set("builder", "claude-code", false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(p.Raisq, "plugins", "claude-code.yaml")); string(data) != mine {
		t.Fatal("the project's own plugin file must never be overwritten")
	}
}

func TestSetRefusesWhatWouldBreakARun(t *testing.T) {
	p := newProject(t, "builder: claude-code\ncritic: claude-code-critic\n", map[string]string{
		"claude-code":        "name: claude-code\nslot: builder\nrun: x\n",
		"claude-code-critic": "name: claude-code-critic\nslot: model\nrun: x\n",
	})
	before, _ := os.ReadFile(ConfigPath(p.Raisq))
	for _, c := range [][2]string{
		{"critic", "claude-code"}, // a builder can't be the critic
		{"builder", "nope"},
		{"attempts", "0"},
		{"batch", "8"},
		{"keep_artifacts", "30"},
		{"spot_check", "2"},
		{"isolation", "vm"},
		{"colour", "red"},
	} {
		if _, err := p.Set(c[0], c[1], false); err == nil {
			t.Errorf("%s %s should be refused", c[0], c[1])
		}
	}
	if _, err := p.Set("isolation", "none", false); err == nil {
		t.Error("isolation none needs a person's yes")
	}
	if after, _ := os.ReadFile(ConfigPath(p.Raisq)); string(after) != string(before) {
		t.Fatalf("a refused change must leave raisq.yaml as it was:\n%s", after)
	}
	if _, err := p.Unset("builder"); err == nil {
		t.Error("the builder has no default to go back to")
	}
}

// 0 is a real choice for spot_check, not "unset".
func TestSpotCheckZeroMeansNone(t *testing.T) {
	p := newProject(t, "builder: x\ncritic: y\n", nil)
	if p.SpotShare() != 0.2 {
		t.Fatal("default share")
	}
	if _, err := p.Set("spot_check", "0", false); err != nil {
		t.Fatal(err)
	}
	if reopen(t, p).SpotShare() != 0 {
		t.Fatal("spot_check 0 must turn spot checks off")
	}
}

func TestRemovePluginKeepsWhatARunNeeds(t *testing.T) {
	p := newProject(t, "builder: claude-code\ncritic: cursor-critic\n", map[string]string{
		"claude-code":   "name: claude-code\nslot: builder\nrun: x\n",
		"cursor-critic": "name: cursor-critic\nslot: model\nrun: x\n",
		"site":          "name: site\nslot: provides\nrun: npm start\n",
		"links":         "name: links\nslot: checks\nrun: builtin:links\nneeds: [site]\n",
	})
	for _, n := range []string{"claude-code", "cursor-critic", "site", "nope"} {
		if _, _, err := p.RemovePlugin(n); err == nil {
			t.Errorf("removing %s should be refused", n)
		}
	}
	if _, _, err := p.RemovePlugin("links"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.RemovePlugin("site"); err != nil {
		t.Fatal("once nothing needs it, a provider can go:", err)
	}
}

// A project set up before local/ moves this machine's state there on first
// open, keeping a person's own .gitignore lines and losing nothing.
func TestOldLayoutMovesToLocal(t *testing.T) {
	dir := t.TempDir()
	raisq := filepath.Join(dir, ".raisq")
	for p, body := range map[string]string{
		"raisq.yaml":                 "builder: claude-code # mine\ncritic: cursor-critic\n",
		"tasks/t1/state.yaml":        "state: ready\nadded: x\n",
		"tasks/t1/spec.yaml":         "name: t1\ntitle: T\ncriteria: [{id: c1, text: x}]\n",
		"history/facts/abc/b.yaml":   "[]\n",
		"baselines/c.json":           "{}",
		"batch-report.md":            "# report\n",
		".gitignore":                 "tasks/*/runs/\ntasks/*/history/\nhistory/\nbaselines/\nmy-own-line\n",
		"facts/business.yaml":        "- id: business/name\n  text: Shop\n  tag: confirmed\n",
		"plugins/cursor-critic.yaml": "name: cursor-critic\nslot: model\nrun: x\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(raisq, p)), 0o755)
		os.WriteFile(filepath.Join(raisq, p), []byte(body), 0o644)
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Config.Builder != "claude-code" || len(p.Moved) == 0 {
		t.Fatalf("settings should load from their new place: %+v moved %v", p.Config, p.Moved)
	}
	if st, err := p.Store.LoadState("t1"); err != nil || st.State != "ready" {
		t.Fatalf("tasks must survive the move: %v %+v", err, st)
	}
	for _, gone := range []string{"raisq.yaml", "tasks", "history", "baselines", "batch-report.md"} {
		if _, err := os.Stat(filepath.Join(raisq, gone)); err == nil {
			t.Errorf("%s should have moved into local/", gone)
		}
	}
	for _, kept := range []string{"facts/business.yaml", "plugins/cursor-critic.yaml", "local/snapshots/facts/abc/b.yaml", "local/baselines/c.json", "local/batch-report.md"} {
		if _, err := os.Stat(filepath.Join(raisq, kept)); err != nil {
			t.Errorf("%s should exist after the move", kept)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(raisq, ".gitignore")); string(data) != "my-own-line\n" {
		t.Fatalf("only RaisQ's own ignore lines go: %q", data)
	}
	if data, _ := os.ReadFile(ConfigPath(raisq)); !strings.Contains(string(data), "# mine") {
		t.Fatal("the config moves as it was, comments included")
	}
	if q := reopen(t, p); len(q.Moved) != 0 {
		t.Fatal("the move happens once")
	}
}

// A teammate who clones a project has its shared files but no settings for
// their own machine: every command says to run raisq init.
func TestClonedProjectNeedsThisMachineSetUp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".raisq", "facts"), 0o755)
	if _, err := Open(dir); err != ErrNoMachine {
		t.Fatalf("want ErrNoMachine, got %v", err)
	}
}

func gitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, c := range []string{"init -q -b main", "config user.email t@t", "config user.name t", "add -A", "commit -qm init --allow-empty"} {
		cmd := exec.Command("git", strings.Fields(c)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s", c, out)
		}
	}
}

// What an agent drafts is tested before it can judge a task: it must fill
// its slot's contract, have the providers it needs, and is run once on the
// project, with nothing saved until a person agrees.
func TestPrepareTestsBeforeAdding(t *testing.T) {
	p := newProject(t, "builder: claude-code\ncritic: cursor-critic\nisolation: none\n", map[string]string{
		"claude-code":   "name: claude-code\nslot: builder\nvendor: a\nrun: x {prompt}\n",
		"cursor-critic": "name: cursor-critic\nslot: model\nvendor: b\nrun: x\n",
	})
	os.WriteFile(filepath.Join(p.Dir, "ok.txt"), []byte("ok\n"), 0o644)
	gitRepo(t, p.Dir)
	file := func(name, body string) string {
		path := filepath.Join(t.TempDir(), name+".yaml")
		os.WriteFile(path, []byte(body), 0o644)
		return path
	}
	ctx := context.Background()
	one := func(src ...string) Candidate {
		t.Helper()
		cs, err := p.Prepare(ctx, src...)
		if err != nil {
			t.Fatal(err)
		}
		return cs[0]
	}

	c := one(file("bad", "name: bad\nslot: checks\nrun: <command>\n"))
	if c.Ready() {
		t.Fatalf("a template left unfilled must not be ready: %+v", c)
	}
	if _, err := p.Add(c); err == nil {
		t.Fatal("a plugin that fails its contract is never saved")
	}

	c = one(file("needy", "name: needy\nslot: checks\nrun: true\nneeds: [cache]\n"))
	if c.Ready() || !strings.Contains(strings.Join(c.Problems, " "), "cache") {
		t.Fatalf("a check needing a provider nobody adds must say so: %+v", c.Problems)
	}
	os.WriteFile(filepath.Join(p.Dir, "cache.env"), []byte("CACHE=up\n"), 0o644)
	cs, err := p.Prepare(ctx, file("needy", "name: needy\nslot: checks\nrun: test \"$CACHE\" = up\nneeds: [cache]\n"),
		file("cache", "name: cache\nslot: provides\nfrom: host:cache.env\n"))
	if err != nil || !cs[0].Ready() || cs[0].Trial == nil || cs[0].Trial.Status != "pass" {
		t.Fatalf("a check added with its provider is tried with it: %v %+v", err, cs)
	}

	c = one(file("good", "name: good\nslot: checks\nrun: grep -q ok ok.txt\n"))
	if !c.Ready() || c.Trial == nil || c.Trial.Status != "pass" {
		t.Fatalf("a working check is tried and passes: %+v %+v", c, c.Trial)
	}
	if _, err := os.Stat(filepath.Join(p.Raisq, "plugins", "good.yaml")); err == nil {
		t.Fatal("preparing saves nothing")
	}
	c = one(file("old", "name: old\nslot: checks\nrun: grep -q missing ok.txt\n"))
	if c.Trial == nil || c.Trial.Status != "fail" || !strings.Contains(c.Trial.Verdict(), "already fails") {
		t.Fatalf("a check failing before any task says it can't catch anything yet: %+v", c.Trial)
	}

	if _, err := p.Add(one(file("good", "name: good\nslot: checks\nrun: grep -q ok ok.txt\n"))); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Prepare(ctx, file("good2", "name: good\nslot: checks\nrun: true\n")); err == nil {
		t.Fatal("a second plugin with the same name must be refused, never overwrite")
	}
	if c := one("links"); !c.BuiltIn {
		t.Fatalf("a built-in is added by its name: %+v", c)
	}
}

// A rubric file is added whole or not at all: one bad rubric, or one whose id
// the project already has, adds none of it.
func TestAddRubricsAllOrNothing(t *testing.T) {
	p := newProject(t, "builder: x\ncritic: y\n", nil)
	file := func(body string) string {
		path := filepath.Join(t.TempDir(), "api.yaml")
		os.WriteFile(path, []byte(body), 0o644)
		return path
	}
	good := "- id: api/auth\n  ask: Is every new endpoint behind an auth guard?\n  fail_if: no\n"
	if _, _, err := p.AddRubrics(file(good + "- id: api/bad\n  ask: x\n  fail_if: maybe\n")); err == nil {
		t.Fatal("fail_if must be yes or no")
	}
	if rs, _ := p.Store.LoadRubrics(); len(rs) != 0 {
		t.Fatalf("a file with a bad rubric adds none of it: %+v", rs)
	}
	if _, _, err := p.AddRubrics(file(good)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.AddRubrics(file(good)); err == nil {
		t.Fatal("a rubric whose id the project has must be refused")
	}
	if _, rs, err := p.AddRubrics("web"); err != nil || len(rs) == 0 {
		t.Fatalf("a built-in set is added by its name: %v", err)
	}
}

// A check that runs tests without listing them leaves the builder free to
// edit those tests, so adding it says so.
func TestWarnsWhenTestsArentProtected(t *testing.T) {
	if w := warnings(command.File{Slot: "checks", Run: "sh test/check.sh"}); len(w) != 1 {
		t.Fatalf("a check protecting nothing must say so, whatever it runs: %v", w)
	}
	if w := warnings(command.File{Slot: "checks", Run: "jest", Tests: []string{"**/*.spec.ts"}}); len(w) != 1 {
		t.Fatalf("tests without run_one can't be proven: %v", w)
	}
	ok := command.File{Slot: "checks", Run: "pnpm exec jest", RunOne: "pnpm exec jest {file}", Tests: []string{"**/*.spec.ts"}}
	if w := warnings(ok); len(w) != 0 {
		t.Fatalf("a protected test check has nothing to warn about: %v", w)
	}
	if w := warnings(command.File{Slot: "checks", Run: "builtin:links"}); len(w) != 0 {
		t.Fatalf("a built-in check judges on its own: %v", w)
	}
	c := Candidate{File: command.File{Slot: "checks", Dir: "apps/api", Tests: []string{"**/*.spec.ts"}, Locks: []string{"jest.config.*"}}}
	if got := strings.Join(c.Protects(), " "); got != "apps/api/**/*.spec.ts apps/api/jest.config.*" {
		t.Fatalf("protects is relative to the project: %s", got)
	}
}

// With isolation: none, a service still runs in a container through the
// host's Docker; with no Docker, it says so instead of running checks
// without it.
func TestLocalServiceNeedsAContainerTool(t *testing.T) {
	saved := containerTool
	containerTool = func() string { return "" }
	defer func() { containerTool = saved }()
	s, _ := Local{}.Start(context.Background(), slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir()})
	_, err := s.(slot.Servicer).Service(context.Background(), slot.ServiceRequest{Name: "postgres", Image: "postgres:16", Port: 5432})
	if err == nil || !strings.Contains(err.Error(), "Docker or Podman") {
		t.Fatalf("want an error saying to start Docker, got %v", err)
	}
}

// With isolation: none, a provider's container runs on the host's Docker, on
// a local port only, reachable from the host session, and gone at the end.
func TestLocalServiceOnHostDocker(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	if containerTool() == "" {
		t.Skip("no Docker or Podman running")
	}
	ctx := context.Background()
	s, _ := Local{}.Start(ctx, slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir()})
	addr, err := s.(slot.Servicer).Service(ctx, slot.ServiceRequest{Name: "cache", Image: "redis:7-alpine", Port: 6379})
	if err != nil {
		t.Fatal(err)
	}
	if addr.Host != "127.0.0.1" || addr.Port == 0 || addr.Port == 6379 {
		t.Fatalf("on the host, a service is on a local port RaisQ picked: %+v", addr)
	}
	name := s.(*localSession).svcs[0][1]
	s.Stop(ctx)
	if exec.Command("docker", "inspect", name).Run() == nil {
		t.Fatal("the service must be removed when the session stops")
	}
}
