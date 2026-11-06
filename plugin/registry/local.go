package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/kkurno/raisq/plugin/docker"
	"github.com/kkurno/raisq/plugin/proc"
	"github.com/kkurno/raisq/slot"
)

// Local runs sessions on the host, for `isolation: none`, a person's choice.
// It's recorded on every review pack. A service the checks need still runs in
// a container, through the host's Docker or Podman when there's one.
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
	// svcs are service containers this session started: tool and name.
	svcs [][2]string
}

// containerTool is the host's Docker or Podman, or "" with neither.
var containerTool = func() string {
	for _, bin := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(bin); err == nil {
			if exec.Command(bin, "info").Run() == nil {
				return bin
			}
		}
	}
	return ""
}

// Service starts a container from an image on the host, its port published
// on a free port of 127.0.0.1 only, and removed when the session stops.
func (s *localSession) Service(ctx context.Context, req slot.ServiceRequest) (slot.Service, error) {
	bin := containerTool()
	if bin == "" {
		return slot.Service{}, fmt.Errorf("%s needs Docker or Podman to start its container (%s), and neither is running here. Start one, or remove the checks that need it", req.Name, req.Image)
	}
	if req.Port == 0 {
		return slot.Service{}, fmt.Errorf("%s needs port: the port %s listens on, so the checks can reach it", req.Name, req.Image)
	}
	name := fmt.Sprintf("raisq-%s-%d", req.Name, os.Getpid())
	args := []string{"run", "-d", "--rm", "--name", name, "-p", fmt.Sprintf("127.0.0.1::%d", req.Port), "--security-opt", "no-new-privileges"}
	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, req.Image)
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		return slot.Service{}, fmt.Errorf("starting %s (%s): %s", req.Name, req.Image, strings.TrimSpace(string(out)))
	}
	s.mu.Lock()
	s.svcs = append(s.svcs, [2]string{bin, name})
	s.mu.Unlock()
	out, err := exec.CommandContext(ctx, bin, "port", name, fmt.Sprintf("%d/tcp", req.Port)).Output()
	if err != nil {
		return slot.Service{}, fmt.Errorf("finding %s's port: %w", req.Name, err)
	}
	_, port, ok := strings.Cut(strings.TrimSpace(strings.Split(string(out), "\n")[0]), ":")
	n, perr := strconv.Atoi(port)
	if !ok || perr != nil {
		return slot.Service{}, fmt.Errorf("finding %s's port: unexpected %q", req.Name, out)
	}
	return slot.Service{Address: slot.Address{Host: "127.0.0.1", Port: n}, Exec: docker.ContainerExec(bin, name)}, nil
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
	for _, svc := range s.svcs {
		_ = exec.Command(svc[0], "rm", "-f", svc[1]).Run()
	}
	return nil
}
