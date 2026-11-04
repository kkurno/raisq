// Package fake fills every slot with something that does nothing real, so the
// core can be tested with no agent, no Docker and no network.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/slot"
)

// Builder runs a function against the checkout on the host, as an agent would
// inside a container.
type Builder struct {
	Fn    func(attempt int, dir, prompt string) (domain.Result, []domain.Choice)
	calls int
	mu    sync.Mutex
}

func (b *Builder) Name() string { return "fake-builder" }

func (b *Builder) Build(ctx context.Context, s slot.Session, prompt string) error {
	b.mu.Lock()
	b.calls++
	n := b.calls
	b.mu.Unlock()
	res, choices := b.Fn(n, s.Paths().Workdir, prompt)
	if err := writeJSON(filepath.Join(s.Paths().Out, "result.json"), res); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.Paths().Out, "choices.json"), choices)
}

func (b *Builder) Calls() int { return b.calls }

// Model answers by the kind of prompt it gets.
type Model struct {
	Fn      func(prompt string) string
	mu      sync.Mutex
	Prompts []string
}

func (m *Model) Name() string { return "fake-model" }

func (m *Model) Ask(ctx context.Context, prompt string) (string, error) {
	m.mu.Lock()
	m.Prompts = append(m.Prompts, prompt)
	m.mu.Unlock()
	return m.Fn(prompt), nil
}

// Isolation runs commands on the host in the checkout. Tests only.
type Isolation struct{}

func (Isolation) Name() string                        { return "fake-isolation" }
func (Isolation) Available(ctx context.Context) error { return nil }

func (Isolation) Start(ctx context.Context, req slot.StartRequest) (slot.Session, error) {
	if err := os.MkdirAll(req.OutDir, 0o755); err != nil {
		return nil, err
	}
	return &Session{paths: slot.Paths{Workdir: req.Checkout, Spec: req.SpecDir, Out: req.OutDir}}, nil
}

type Session struct {
	paths slot.Paths
	Ran   []string
}

func (s *Session) Paths() slot.Paths { return s.paths }

func (s *Session) Exec(ctx context.Context, cmd string, env map[string]string) (slot.ExecResult, error) {
	s.Ran = append(s.Ran, cmd)
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = s.paths.Workdir
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}
	out, err := c.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return slot.ExecResult{Exit: ee.ExitCode(), Output: string(out)}, nil
	}
	return slot.ExecResult{Output: string(out)}, err
}

func (s *Session) Background(ctx context.Context, cmd string, env map[string]string) error {
	return nil
}
func (s *Session) URL(port int) (string, error)   { return fmt.Sprintf("http://127.0.0.1:%d", port), nil }
func (s *Session) Stop(ctx context.Context) error { return nil }

// Check runs a shell command, like the command plugin would.
type Check struct {
	CheckName string
	Cmd       string
	Lock      []string
	TestGlob  []string
	OneCmd    string // with {file}
	Need      []string
}

func (c Check) Name() string    { return c.CheckName }
func (c Check) Priority() int   { return 0 }
func (c Check) Needs() []string { return c.Need }
func (c Check) Locks() []string { return c.Lock }
func (c Check) Tests() []string { return c.TestGlob }

func (c Check) Run(ctx context.Context, s slot.Session, env map[string]string) domain.CheckResult {
	return runCmd(ctx, s, env, c.CheckName, c.Cmd)
}

func (c Check) RunOne(ctx context.Context, s slot.Session, env map[string]string, file string) (domain.CheckResult, bool) {
	if c.OneCmd == "" {
		return domain.CheckResult{}, false
	}
	return runCmd(ctx, s, env, c.CheckName, strings.ReplaceAll(c.OneCmd, "{file}", file)), true
}

func runCmd(ctx context.Context, s slot.Session, env map[string]string, name, cmd string) domain.CheckResult {
	r, err := s.Exec(ctx, cmd, env)
	if err != nil {
		return domain.CheckResult{Name: name, Status: domain.CheckBlocked, Output: err.Error()}
	}
	if r.Exit == 127 {
		return domain.CheckResult{Name: name, Status: domain.CheckBlocked, Output: r.Output}
	}
	if r.Exit != 0 {
		return domain.CheckResult{Name: name, Status: domain.CheckFail, Output: r.Output}
	}
	return domain.CheckResult{Name: name, Status: domain.CheckPass, Output: r.Output}
}

// Report keeps what it was given.
type Report struct {
	Items []slot.ReportItem
}

func (r *Report) Name() string { return "fake-report" }
func (r *Report) Deliver(ctx context.Context, it slot.ReportItem) error {
	r.Items = append(r.Items, it)
	return nil
}

func writeJSON(p string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
