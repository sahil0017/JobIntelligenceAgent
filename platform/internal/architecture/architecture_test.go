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

const modulePath = "careerpilot/"

// allowed lists, per top-level layer, the layers it may import. Anything else in this module is rejected.
var allowed = map[string][]string{
	"contracts":    {},
	"intelligence": {"contracts"},
	"platform":     {"contracts", "intelligence"},
}

// forbiddenInternal maps a platform/internal package to the sibling packages it must never import.
var forbiddenInternal = map[string][]string{
	"storage":      {"api", "app", "integrations", "services"},
	"integrations": {"api", "app"},
	"services":     {"api", "app"},
	"api":          {"app"},
}

func TestLayersRespectBoundaries(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	fset := token.NewFileSet()
	for layer := range allowed {
		err := filepath.WalkDir(filepath.Join(root, layer), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return walkErr
			}
			file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			relative, _ := filepath.Rel(root, path)
			relative = filepath.ToSlash(relative)
			for _, spec := range file.Imports {
				imported := strings.Trim(spec.Path.Value, `"`)
				if !strings.HasPrefix(imported, modulePath) {
					continue
				}
				target := strings.Split(strings.TrimPrefix(imported, modulePath), "/")[0]
				if target != layer && !contains(allowed[layer], target) {
					t.Errorf("%s imports %s: layer %q must not depend on %q", relative, imported, layer, target)
				}
				if layer == "platform" {
					checkInternal(t, relative, imported)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func checkInternal(t *testing.T, relative, imported string) {
	const prefix = "platform/internal/"
	if !strings.HasPrefix(relative, prefix) || !strings.HasPrefix(imported, modulePath+prefix) {
		return
	}
	module := strings.Split(strings.TrimPrefix(relative, prefix), "/")[0]
	target := strings.Split(strings.TrimPrefix(imported, modulePath+prefix), "/")[0]
	if target != module && contains(forbiddenInternal[module], target) {
		t.Errorf("%s imports %s: %q modules must not depend on %q", relative, imported, module, target)
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
