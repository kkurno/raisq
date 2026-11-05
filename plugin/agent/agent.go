// Package agent fills the builder slot by running an agent CLI inside the
// session. What it prints never decides anything: the result is result.json
// and the files. The one thing read from it is whether the vendor's quota ran
// out, which stops a batch and judges nothing.
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kkurno/raisq/plugin/proc"
	"github.com/kkurno/raisq/slot"
)

type Agent struct {
	PluginName string
	// Run is the command, with {prompt} for the prompt file inside the session.
	Run string
	// Install puts the agent CLI into a fresh container when it isn't there.
	Install string
	Find    string
	Timeout time.Duration
}

func (a Agent) Name() string { return a.PluginName }

func (a Agent) Build(ctx context.Context, s slot.Session, prompt string) error {
	out := s.Paths().Out
	host := hostOut(s)
	if err := os.WriteFile(filepath.Join(host, "prompt.md"), []byte(prompt), 0o644); err != nil {
		return err
	}
	if a.Install != "" {
		find := a.Find
		if find == "" {
			find = "false"
		}
		if r, err := s.Exec(ctx, find+" >/dev/null 2>&1 || ("+a.Install+")", nil); err != nil || r.Exit != 0 {
			return fmt.Errorf("installing %s in the container: %s", a.PluginName, r.Output)
		}
	}
	if a.Timeout == 0 {
		a.Timeout = 60 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	cmd := strings.ReplaceAll(a.Run, "{prompt}", filepath.Join(out, "prompt.md"))
	r, err := s.Exec(ctx, cmd, nil)
	// The transcript is kept for a person; nothing in RaisQ reads it.
	_ = os.WriteFile(filepath.Join(host, "transcript.log"), []byte(r.Output), 0o644)
	if err != nil {
		return err
	}
	if r.Exit != 0 {
		if proc.UsageLimited(r.Output) {
			return slot.ErrUsageLimit
		}
		return fmt.Errorf("%s exited %d", a.PluginName, r.Exit)
	}
	return nil
}

// hostOut is where the session's out directory is on the host.
func hostOut(s slot.Session) string {
	if h, ok := s.(interface{ HostOut() string }); ok {
		return h.HostOut()
	}
	return s.Paths().Out
}
