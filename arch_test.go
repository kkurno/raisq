package raisq_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDependencyRule fails the build on any import pointing outwards: the
// rules import nothing, the flows import only the rules, plugins come last.
func TestDependencyRule(t *testing.T) {
	allowed := map[string][]string{
		"domain":  {},
		"slot":    {"raisq/domain"},
		"usecase": {"raisq/domain", "raisq/slot"},
	}
	for ring, ok := range allowed {
		err := filepath.Walk(ring, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			test := strings.HasSuffix(p, "_test.go")
			f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if !strings.HasPrefix(path, "raisq/") {
					continue
				}
				// Tests may use fakes and real plugins to exercise a ring.
				if test || strings.HasPrefix(p, "slot/fake") {
					continue
				}
				if !within(path, ok) {
					t.Errorf("%s imports %s: %s may only import %v", p, path, ring, ok)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func within(path string, ok []string) bool {
	for _, o := range ok {
		if path == o || strings.HasPrefix(path, o+"/") {
			return true
		}
	}
	return false
}
