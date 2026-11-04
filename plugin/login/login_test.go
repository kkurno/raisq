package login

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func creds(expires time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"oauth":    map[string]any{"accessToken": "short", "refreshToken": "long-lived", "expiresAt": expires.UnixMilli()},
		"mcpOAuth": map[string]any{"jira": map[string]any{"accessToken": "jira-token", "clientSecret": "s3cret"}},
	})
	return string(b)
}

func TestFileFirstThenCommand(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "c.json")
	s := Source{File: f, Command: "echo from-command"}
	got, err := s.Read(context.Background())
	if err != nil || string(got) != "from-command" {
		t.Fatalf("no file: the command is used: %q %v", got, err)
	}
	os.WriteFile(f, []byte("from-file"), 0o600)
	got, _ = s.Read(context.Background())
	if string(got) != "from-file" {
		t.Fatal("the file wins when it's there")
	}
	if _, err := (Source{File: filepath.Join(dir, "none")}).Read(context.Background()); err != ErrNotFound {
		t.Fatal("nothing found is ErrNotFound")
	}
}

func TestOnlyKeptFieldsGoIn(t *testing.T) {
	s := Source{Command: "echo '" + creds(time.Now().Add(5*time.Hour)) + "'",
		Keep: []string{"oauth.accessToken", "oauth.expiresAt"}, ExpiresAt: "oauth.expiresAt"}
	got, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, never := range []string{"long-lived", "jira-token", "s3cret", "mcpOAuth"} {
		if strings.Contains(string(got), never) {
			t.Fatalf("%q must stay on this machine: %s", never, got)
		}
	}
	if !strings.Contains(string(got), "short") || !strings.Contains(string(got), "expiresAt") {
		t.Fatalf("the kept fields go in: %s", got)
	}
}

func TestNearExpiryIsRenewedFirst(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "c.json")
	os.WriteFile(f, []byte(creds(time.Now().Add(10*time.Minute))), 0o600)
	fresh := filepath.Join(dir, "fresh.json")
	os.WriteFile(fresh, []byte(creds(time.Now().Add(8*time.Hour))), 0o600)
	s := Source{File: f, ExpiresAt: "oauth.expiresAt", Refresh: "cp " + fresh + " " + f}
	if _, err := s.Read(context.Background()); err != nil {
		t.Fatalf("a login that would expire mid-attempt is renewed: %v", err)
	}
	os.WriteFile(f, []byte(creds(time.Now().Add(10*time.Minute))), 0o600)
	if _, err := (Source{File: f, ExpiresAt: "oauth.expiresAt"}).Read(context.Background()); err == nil {
		t.Fatal("with no way to renew it, a nearly expired login is refused")
	}
}

func TestShortForm(t *testing.T) {
	var ss []Source
	if err := yaml.Unmarshal([]byte("- \"~/.x/auth.json:/home/raisq/.x/auth.json\"\n- to: /a\n  command: echo hi\n"), &ss); err != nil {
		t.Fatal(err)
	}
	if ss[0].File != "~/.x/auth.json" || ss[0].To != "/home/raisq/.x/auth.json" || ss[1].Command != "echo hi" {
		t.Fatalf("%+v", ss)
	}
}
