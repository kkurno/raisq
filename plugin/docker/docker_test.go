package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkurno/raisq/plugin/command"
	loginpkg "github.com/kkurno/raisq/plugin/login"
	"github.com/kkurno/raisq/slot"
	"github.com/kkurno/raisq/usecase"
)

// TestIsolation needs a running Docker; it's skipped without one.
func TestIsolation(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("alpine:3.20")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	if out, err := exec.Command("docker", "pull", "-q", "alpine:3.20").CombinedOutput(); err != nil {
		t.Skip(string(out))
	}
	work, spec, out, login := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(spec, "INDEX.md"), []byte("facts"), 0o644)
	os.WriteFile(filepath.Join(login, "settings.json"), []byte("{}"), 0o600)
	d.Login = []loginpkg.Source{{File: login, To: "/home/raisq/.agent"}}
	s, err := d.Start(ctx, slot.StartRequest{Checkout: work, SpecDir: spec, OutDir: out, Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)

	r, err := s.Exec(ctx, "echo hi > /work/a.txt && cat /raisq/spec/INDEX.md", nil)
	if err != nil || r.Exit != 0 || !strings.Contains(r.Output, "facts") {
		t.Fatalf("exec: %v %+v", err, r)
	}
	if data, _ := os.ReadFile(filepath.Join(work, "a.txt")); string(data) != "hi\n" {
		t.Fatal("the checkout is mounted read-write")
	}
	if r, _ := s.Exec(ctx, "echo x > /raisq/spec/INDEX.md", nil); r.Exit == 0 {
		t.Fatal("the spec must be read-only")
	}
	if r, _ := s.Exec(ctx, `cat /home/raisq/.agent/settings.json && echo '{"hooks":"curl evil"}' > /home/raisq/.agent/settings.json`, nil); r.Exit != 0 {
		t.Fatalf("the login should be readable inside: %s", r.Output)
	}
	if data, _ := os.ReadFile(filepath.Join(login, "settings.json")); string(data) != "{}" {
		t.Fatal("a write to the login inside must never reach the host")
	}
	home, _ := os.UserHomeDir()
	if r, _ := s.Exec(ctx, "ls "+home, nil); r.Exit == 0 {
		t.Fatal("the user's home must not be inside")
	}
}

// A login variable that's set replaces the copied login, so only one
// credential is ever inside.
func TestLoginEnvReplacesTheCopiedLogin(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("alpine:3.20")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	login := t.TempDir()
	os.WriteFile(filepath.Join(login, "token.json"), []byte(`{"a":1}`), 0o600)
	d.Login = []loginpkg.Source{{File: filepath.Join(login, "token.json"), To: "/home/raisq/.agent/token.json"}}
	d.LoginEnv = []string{"RAISQ_TEST_TOKEN"}
	t.Setenv("RAISQ_TEST_TOKEN", "from-env")
	s, err := d.Start(ctx, slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir(), Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	if r, _ := s.Exec(ctx, "test -e /home/raisq/.agent/token.json", nil); r.Exit == 0 {
		t.Fatal("with a login variable set, the login file must not be copied in too")
	}
	if r, _ := s.Exec(ctx, `printf %s "$RAISQ_TEST_TOKEN"`, nil); r.Output != "from-env" {
		t.Fatalf("the login variable should be inside, got %q", r.Output)
	}
}

// A pnpm project gets pnpm at the version its packageManager names, which a
// Node image doesn't ship and the unprivileged user can't install itself.
func TestPackageManagerFromThePackage(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("node:22-bookworm")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "package.json"), []byte(`{"name":"x","packageManager":"pnpm@9.12.0"}`), 0o644)
	s, err := d.Start(ctx, slot.StartRequest{Checkout: work, OutDir: t.TempDir(), Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	r, err := s.Exec(ctx, "pnpm --version", nil)
	if err != nil || r.Exit != 0 || !strings.Contains(r.Output, "9.12.0") {
		t.Fatalf("want pnpm 9.12.0 in the container, got %v %+v", err, r)
	}
}

// A provider with image: runs a service beside the session for the checks,
// reachable by the provider's name, and gone when the session stops.
func TestServiceBesideTheSession(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("alpine:3.20")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	s, err := d.Start(ctx, slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir(), Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	sess := s.(*Session)
	svc, err := sess.Service(ctx, slot.ServiceRequest{Name: "cache", Image: "redis:7-alpine", Port: 6379})
	if err != nil {
		t.Fatal(err)
	}
	if svc.Host != "cache" || svc.Port != 6379 {
		t.Fatalf("in a container, a service is at its name and own port: %+v", svc.Address)
	}
	ok := false
	for i := 0; i < 30 && !ok; i++ {
		r, _ := s.Exec(ctx, `printf 'PING\r\n' | nc -w 2 cache 6379`, nil)
		ok = strings.Contains(r.Output, "PONG")
		if !ok {
			time.Sleep(time.Second)
		}
	}
	if !ok {
		t.Fatal("the service should answer at its name from the session")
	}
	container, network := sess.svcs[0], sess.net
	s.Stop(ctx)
	if exec.Command("docker", "inspect", container).Run() == nil {
		t.Fatal("the service must stop with the session")
	}
	if exec.Command("docker", "network", "inspect", network).Run() == nil {
		t.Fatal("the session's network must go with it")
	}
}

// A package that uses bun gets bun in the container, at the version its
// packageManager names, through the command RaisQ runs for bun.lock.
func TestBunFromThePackage(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("node:22-bookworm")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "package.json"), []byte(`{"name":"auth","packageManager":"bun@1.1.38"}`), 0o644)
	os.WriteFile(filepath.Join(work, "bun.lock"), []byte(`{"lockfileVersion":1,"workspaces":{"":{"name":"auth"}},"packages":{}}`), 0o644)
	s, err := d.Start(ctx, slot.StartRequest{Checkout: work, OutDir: t.TempDir(), Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	if r, err := s.Exec(ctx, usecase.InstallCommand("bun.lock"), nil); err != nil || r.Exit != 0 {
		t.Fatalf("installing with bun: %v %+v", err, r)
	}
	r, err := s.Exec(ctx, "bun --version", nil)
	if err != nil || r.Exit != 0 || !strings.Contains(r.Output, "1.1.38") {
		t.Fatalf("want bun 1.1.38 in the container, got %v %+v", err, r)
	}
}

// A Postgres provider becomes ready by its image's own pg_isready, run inside
// its container: the checks' container has no Postgres tools.
func TestPostgresProviderReadyInItsContainer(t *testing.T) {
	if os.Getenv("RAISQ_DOCKER_TEST") == "" {
		t.Skip("set RAISQ_DOCKER_TEST=1 to run against Docker")
	}
	ctx := context.Background()
	d := New("alpine:3.20")
	if err := d.Available(ctx); err != nil {
		t.Skip(err)
	}
	s, err := d.Start(ctx, slot.StartRequest{Checkout: t.TempDir(), OutDir: t.TempDir(), Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop(ctx)
	p := command.Provider{F: command.File{Name: "postgres", Slot: "provides", Image: "postgres:16-alpine", Port: 5432,
		Env: map[string]string{"POSTGRES_PASSWORD": "test"}, Ready: "pg_isready -U postgres"}}
	if _, err := p.Start(ctx, s); err != nil {
		t.Fatalf("postgres should become ready by its own pg_isready: %v", err)
	}
	if r, _ := s.Exec(ctx, "command -v pg_isready", nil); r.Exit == 0 {
		t.Fatal("the checks' container has no pg_isready, so the probe can only have run in the service")
	}
}
