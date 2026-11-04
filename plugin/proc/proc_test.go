package proc

import (
	"context"
	"testing"
	"time"
)

func TestTimeoutStopsTheWholeGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	// A child that ignores its parent dying and keeps stdout open, as an agent
	// CLI started from sh -c can.
	start := time.Now()
	_, _ = Shell(ctx, `sleep 30 & wait`).Output()
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("the timeout didn't stop it: waited %s", d)
	}
}
