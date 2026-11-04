package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	loginpkg "github.com/kkurno/raisq/plugin/login"
	"github.com/kkurno/raisq/slot"
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
