// Package architecture enforces the module boundaries described in docs/ARCHITECTURE.md.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "careerpilot/backend/internal/"

// forbidden maps a package directory (relative to internal/) to the packages it must never import.
var forbidden = map[string][]string{
	"domain":       {"api", "app", "agent", "storage", "integrations", "services"},
	"storage":      {"api", "app", "agent", "integrations", "services"},
	"integrations": {"api", "app", "agent"},
	"services":     {"api", "app"},
	"agent":        {"api", "app"},
	"api":          {"app"},
}

func TestPackagesRespectModuleBoundaries(t *testing.T) {
	root := ".."
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		module := strings.Split(filepath.ToSlash(relative), "/")[0]
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if !strings.HasPrefix(imported, modulePrefix) {
				continue
			}
			target := strings.Split(strings.TrimPrefix(imported, modulePrefix), "/")[0]
			for _, banned := range forbidden[module] {
				if target == banned && target != module {
					t.Errorf("%s imports %s: %q modules must not depend on %q", relative, imported, module, banned)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
