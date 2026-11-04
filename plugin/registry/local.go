package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/kkurno/raisq/plugin/proc"
	"github.com/kkurno/raisq/slot"
)

// Local runs sessions on the host, for `isolation: none` on a machine that's
// disposable anyway. It's recorded on every review pack.
type Local struct{}

func (Local) Name() string                        { return "none" }
func (Local) Available(ctx context.Context) error { return nil }

func (Local) Start(ctx context.Context, req slot.StartRequest) (slot.Session, error) {
	if err := os.MkdirAll(req.OutDir, 0o755); err != nil {
		return nil, err
	}
	return &localSession{paths: slot.Paths{Workdir: req.Checkout, Spec: req.SpecDir, Out: req.OutDir}}, nil
}

type localSession struct {
	paths slot.Paths
	mu    sync.Mutex
	bg    []*exec.Cmd
}

func (s *localSession) Paths() slot.Paths { return s.paths }

func (s *localSession) cmd(ctx context.Context, line string, env map[string]string) *exec.Cmd {
	c := proc.Shell(ctx, line)
	c.Dir = s.paths.Workdir
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}
	return c
}

func (s *localSession) Exec(ctx context.Context, line string, env map[string]string) (slot.ExecResult, error) {
	out, err := s.cmd(ctx, line, env).CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return slot.ExecResult{Exit: ee.ExitCode(), Output: string(out)}, nil
	}
	return slot.ExecResult{Output: string(out)}, err
}

func (s *localSession) Background(ctx context.Context, line string, env map[string]string) error {
	c := s.cmd(context.Background(), line, env)
	if err := c.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	s.bg = append(s.bg, c)
	s.mu.Unlock()
	return nil
}

func (s *localSession) URL(port int) (string, error) {
	return fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

func (s *localSession) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.bg {
		if c.Process != nil {
			c.Process.Kill()
		}
	}
	return nil
}
