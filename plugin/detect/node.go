package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultNode is the Node major for a project that doesn't say.
const DefaultNode = "22"

var firstMajor = regexp.MustCompile(`\d+`)

// ltsNames maps nvm's LTS codenames to their major version.
var ltsNames = map[string]string{
	"hydrogen": "18", "iron": "20", "jod": "22", "krypton": "24",
}

// NodeMajor is the Node major version a project asks for, from .nvmrc,
// .node-version, .tool-versions or package.json's engines.node, in that
// order. It's "" when none of them says.
func NodeMajor(dir string) string {
	for _, name := range []string{".nvmrc", ".node-version"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if v := major(strings.TrimSpace(string(data))); v != "" {
				return v
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, ".tool-versions")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if f := strings.Fields(line); len(f) > 1 && f[0] == "nodejs" {
				if v := major(f[1]); v != "" {
					return v
				}
			}
		}
	}
	var pkg struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && json.Unmarshal(data, &pkg) == nil {
		// The lowest version a range allows is one it accepts: >=24 → 24, ^20 → 20.
		return major(pkg.Engines.Node)
	}
	return ""
}

func major(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if name, ok := strings.CutPrefix(v, "lts/"); ok {
		return ltsNames[name]
	}
	return firstMajor.FindString(v)
}
