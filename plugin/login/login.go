// Package login reads an agent's login on the host, so the isolation plugin
// can copy it into a container. Where the login lives, and what to leave out
// of it, is written in the agent's plugin file; nothing here knows a vendor.
package login

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/plugin/proc"
)

// ValidFor is how long a login must stay valid once it's in a container: an
// attempt's whole run, with room to spare.
const ValidFor = 75 * time.Minute

var ErrNotFound = errors.New("no login found")

// Source is one login file the agent needs inside the container.
type Source struct {
	// To is where the agent expects it inside the container, as a file.
	To string `yaml:"to,omitempty"`
	// Env is the variable it goes in instead, for an agent that reads its
	// login from the environment.
	Env string `yaml:"env,omitempty"`
	// File is where it is on this machine, such as on Linux.
	File string `yaml:"file,omitempty"`
	// Command prints it, for a login kept somewhere else, such as the macOS
	// Keychain. Used when File isn't there.
	Command string `yaml:"command,omitempty"`
	// Keep is the JSON fields that go into the container, and nothing else. A
	// login store can hold other tools' tokens too, so it's a list of what to
	// keep, never of what to drop: an unlisted field stays on this machine.
	Keep []string `yaml:"keep,omitempty"`
	// ExpiresAt is the JSON field with the login's expiry, in seconds or
	// milliseconds since 1970.
	ExpiresAt string `yaml:"expires_at,omitempty"`
	// Refresh renews the login on this machine when it would expire mid-attempt.
	Refresh string `yaml:"refresh,omitempty"`
}

// UnmarshalYAML also takes the short form "host-path:container-path".
func (s *Source) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		host, guest, _ := strings.Cut(n.Value, ":")
		s.File, s.To = host, guest
		return nil
	}
	type plain Source
	return n.Decode((*plain)(s))
}

// Where says where it goes inside, for messages.
func (s Source) Where() string {
	if s.Env != "" {
		return "$" + s.Env
	}
	return s.To
}

// Describe says where it looks, for doctor.
func (s Source) Describe() string {
	var w []string
	if s.File != "" {
		w = append(w, s.File)
	}
	if s.Command != "" {
		w = append(w, "`"+s.Command+"`")
	}
	return strings.Join(w, " or ")
}

// IsDir reports whether the source is a folder on this machine, copied whole.
func (s Source) IsDir() bool {
	info, err := os.Stat(Home(s.File))
	return s.File != "" && err == nil && info.IsDir()
}

// Read returns the login as it goes into the container: renewed if it would
// expire within ValidFor, and with Drop left out.
func (s Source) Read(ctx context.Context) ([]byte, error) {
	data, err := s.raw(ctx)
	if err != nil {
		return nil, err
	}
	if s.ExpiresAt != "" {
		if left, ok := remaining(data, s.ExpiresAt); ok && left < ValidFor {
			if s.Refresh == "" {
				return nil, fmt.Errorf("the login expires in %s, and the plugin says no way to renew it", left.Round(time.Minute))
			}
			if err := run(ctx, s.Refresh, 3*time.Minute); err != nil {
				return nil, fmt.Errorf("renewing the login: %w", err)
			}
			if data, err = s.raw(ctx); err != nil {
				return nil, err
			}
			if left, ok := remaining(data, s.ExpiresAt); ok && left < ValidFor {
				return nil, fmt.Errorf("the login still expires in %s after renewing it", left.Round(time.Minute))
			}
		}
	}
	if len(s.Keep) > 0 {
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("the login isn't JSON, so only %v can't be picked out of it", s.Keep)
		}
		kept := map[string]any{}
		for _, path := range s.Keep {
			copyPath(v, kept, strings.Split(path, "."))
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("none of %v is in the login", s.Keep)
		}
		if data, err = json.Marshal(kept); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// copyPath copies one field from src to dst, making the objects on the way.
func copyPath(src, dst map[string]any, path []string) {
	v, ok := src[path[0]]
	if !ok {
		return
	}
	if len(path) == 1 {
		dst[path[0]] = v
		return
	}
	next, ok := v.(map[string]any)
	if !ok {
		return
	}
	sub, _ := dst[path[0]].(map[string]any)
	if sub == nil {
		sub = map[string]any{}
		dst[path[0]] = sub
	}
	copyPath(next, sub, path[1:])
}

func (s Source) raw(ctx context.Context) ([]byte, error) {
	if s.File != "" {
		data, err := os.ReadFile(Home(s.File))
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, nil
		}
	}
	if s.Command != "" {
		// A login store can ask the person first, such as the Keychain's
		// "allow access" dialog, so this has a limit rather than hanging a batch.
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		out, err := proc.Shell(ctx, s.Command).Output()
		if err == nil && len(bytes.TrimSpace(out)) > 0 {
			return bytes.TrimSpace(out), nil
		}
	}
	return nil, ErrNotFound
}

func remaining(data []byte, path string) (time.Duration, bool) {
	var v map[string]any
	if json.Unmarshal(data, &v) != nil {
		return 0, false
	}
	var cur any = v
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur = m[k]
	}
	n, ok := cur.(float64)
	if !ok {
		return 0, false
	}
	if n > 1e12 {
		n /= 1000
	}
	return time.Until(time.Unix(int64(n), 0)), true
}

func run(ctx context.Context, cmd string, limit time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := proc.Shell(ctx, cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Home expands a leading ~/.
func Home(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}
