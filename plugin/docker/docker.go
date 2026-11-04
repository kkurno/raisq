// Package docker fills the isolation slot with a container per session.
package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"raisq/plugin/login"
	"raisq/slot"
)

type Docker struct {
	Bin   string // docker or podman
	Image string
	// Login is the agent login to bring in, and nothing else from the user's
	// home (5.2). It's copied, never mounted, so nothing the builder writes
	// there reaches the host.
	Login []login.Source
	// LoginEnv names host variables to pass in when set, such as an OAuth
	// token for an agent that keeps its login in the system keychain.
	LoginEnv []string
	Env      map[string]string
}

func New(image string) *Docker { return &Docker{Bin: "docker", Image: image} }

func (d *Docker) Name() string { return d.Bin }

func (d *Docker) Available(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, d.Bin, "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s isn't running: %s", d.Bin, strings.TrimSpace(string(out)))
	}
	return nil
}

const (
	guestWork = "/work"
	guestSpec = "/raisq/spec"
	guestOut  = "/raisq/out"
	// guestHome is where the builder's user lives. Logins are copied here.
	guestHome  = "/home/raisq"
	guestLogin = "/raisq/login"
)

func (d *Docker) Start(ctx context.Context, req slot.StartRequest) (slot.Session, error) {
	name := "raisq-" + sanitize(req.Label) + "-" + randHex(4)
	args := []string{"run", "-d", "--rm", "--name", name,
		"--workdir", guestWork,
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--cap-add", "FOWNER", "--cap-add", "SETUID", "--cap-add", "SETGID",
		"-v", req.Checkout + ":" + guestWork,
		"-v", req.OutDir + ":" + guestOut,
	}
	if req.SpecDir != "" {
		args = append(args, "-v", req.SpecDir+":"+guestSpec+":ro")
	}
	loginDir, logins, err := copyLogin(ctx, d.Login)
	if err != nil {
		return nil, err
	}
	if loginDir != "" {
		// One folder of copies, placed into the user's home by makeUser.
		args = append(args, "-v", loginDir+":"+guestLogin+":ro")
	}
	for _, m := range req.Mounts {
		spec := m.Host + ":" + m.Guest
		if m.ReadOnly {
			spec += ":ro"
		}
		args = append(args, "-v", spec)
	}
	for _, p := range req.Ports {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1::%d", p))
	}
	for k, v := range d.Env {
		args = append(args, "-e", k+"="+v)
	}
	for _, k := range d.LoginEnv {
		if v := os.Getenv(k); v != "" {
			// Passed by name, so the value never appears in a process list.
			args = append(args, "-e", k)
		}
	}
	args = append(args, "-e", "HOME="+guestHome, "-e", "NPM_CONFIG_PREFIX="+guestHome+"/.npm-global",
		"-e", "PATH="+guestHome+"/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	args = append(args, d.Image, "sleep", "infinity")
	if out, err := exec.CommandContext(ctx, d.Bin, args...).CombinedOutput(); err != nil {
		os.RemoveAll(loginDir)
		return nil, fmt.Errorf("starting the container: %s", strings.TrimSpace(string(out)))
	}
	s := &Session{d: d, name: name, host: req.Checkout, out: req.OutDir, login: loginDir, logins: logins}
	if err := s.makeUser(ctx); err != nil {
		s.Stop(ctx)
		return nil, err
	}
	return s, nil
}

// makeUser adds the unprivileged user every command runs as, with the same
// uid as the files on the host so what it writes stays editable there. Agents
// refuse to skip their permission prompts as root, and nothing here needs it.
func (s *Session) makeUser(ctx context.Context) error {
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 {
		uid, gid = 1000, 1000
	}
	script := fmt.Sprintf(`set -e
getent group %[2]d >/dev/null || (groupadd -g %[2]d raisq 2>/dev/null || addgroup -g %[2]d raisq)
if ! getent passwd %[1]d >/dev/null; then
  useradd -m -d %[3]s -u %[1]d -g %[2]d raisq 2>/dev/null || adduser -D -h %[3]s -u %[1]d -G "$(getent group %[2]d | cut -d: -f1)" raisq
fi
mkdir -p %[3]s
chown %[1]d:%[2]d %[3]s %[4]s %[5]s`, uid, gid, guestHome, guestWork, guestOut)
	for _, m := range s.logins {
		script += fmt.Sprintf("\nmkdir -p %[1]s && cp -a %[2]s %[3]s && chown -R %[4]d:%[5]d %[6]s",
			shq(path.Dir(m.Guest)), shq(path.Join(guestLogin, m.Host)), shq(m.Guest), uid, gid, shq(guestHome))
	}
	c := exec.CommandContext(ctx, s.d.Bin, "exec", "-u", "0", s.name, "sh", "-c", script)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("making the container's user: %s", strings.TrimSpace(string(out)))
	}
	s.user = fmt.Sprintf("%d:%d", uid, gid)
	return nil
}

// copyLogin writes each login into a private temporary folder and returns
// where each copy goes inside. A login that isn't found is skipped: doctor
// says so, and the agent says it isn't logged in.
func copyLogin(ctx context.Context, ss []login.Source) (string, []slot.Mount, error) {
	if len(ss) == 0 {
		return "", nil, nil
	}
	dir, err := os.MkdirTemp("", "raisq-login-")
	if err != nil {
		return "", nil, err
	}
	var out []slot.Mount
	for i, src := range ss {
		rel := filepath.Join(fmt.Sprint(i), path.Base(src.To))
		dst := filepath.Join(dir, rel)
		if src.IsDir() {
			err = copyDir(login.Home(src.File), dst)
		} else {
			var data []byte
			if data, err = src.Read(ctx); errors.Is(err, login.ErrNotFound) {
				continue
			} else if err == nil {
				err = writePrivate(dst, data)
			}
		}
		if err != nil {
			os.RemoveAll(dir)
			return "", nil, fmt.Errorf("bringing in the login for %s: %w", src.To, err)
		}
		out = append(out, slot.Mount{Host: filepath.ToSlash(rel), Guest: src.To})
	}
	return dir, out, nil
}

func writePrivate(dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// The copy lives in a private temp folder; inside, it's owned by the
	// container's user (makeUser).
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o700)
		case info.Mode().IsRegular():
			return copyFile(p, target)
		}
		return nil
	})
}

type Session struct {
	d      *Docker
	name   string
	host   string
	out    string
	login  string
	logins []slot.Mount // Host is relative to the login folder
	user   string
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (s *Session) HostOut() string { return s.out }

func (s *Session) Paths() slot.Paths {
	return slot.Paths{Workdir: guestWork, Spec: guestSpec, Out: guestOut}
}

func (s *Session) HostDir() string { return s.host }

func (s *Session) Exec(ctx context.Context, cmd string, env map[string]string) (slot.ExecResult, error) {
	args := []string{"exec", "-i", "-u", s.user}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, s.name, "sh", "-c", cmd)
	c := exec.CommandContext(ctx, s.d.Bin, args...)
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	err := c.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return slot.ExecResult{Exit: ee.ExitCode(), Output: out.String()}, nil
	}
	return slot.ExecResult{Output: out.String()}, err
}

func (s *Session) Background(ctx context.Context, cmd string, env map[string]string) error {
	args := []string{"exec", "-d", "-u", s.user}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, s.name, "sh", "-c", cmd+" >/tmp/raisq-bg.log 2>&1")
	if out, err := exec.CommandContext(ctx, s.d.Bin, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func (s *Session) URL(port int) (string, error) {
	out, err := exec.Command(s.d.Bin, "port", s.name, fmt.Sprint(port)).Output()
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return "http://" + addr, nil
}

func (s *Session) Stop(ctx context.Context) error {
	err := exec.Command(s.d.Bin, "rm", "-f", s.name).Run()
	if s.login != "" {
		os.RemoveAll(s.login)
	}
	return err
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "task"
	}
	return b.String()
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
