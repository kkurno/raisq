package proc

import (
	"context"
	"os/exec"
	"time"
)

// Shell runs a command line so that a timeout or a cancel stops it and
// everything it started: `sh -c "agent ..."` would otherwise leave the agent
// running and holding the output open, and the wait would never end.
func Shell(ctx context.Context, line string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
	setGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	// Once killed, don't wait longer than this for pipes a stray child holds.
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
