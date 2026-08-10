package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProductionPackagesDeclareTheirBoundaries makes package ownership
// mechanically visible. A directory namespace must not become a documentation-
// only umbrella, and every real internal package must name its responsibility
// in GoDoc using the same package name as its directory.
func TestProductionPackagesDeclareTheirBoundaries(t *testing.T) {
	internalRoot := filepath.Join(moduleRoot(t), "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == "testdata" {
			return filepath.SkipDir
		}
		files, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		var production []string
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
				continue
			}
			production = append(production, file.Name())
		}
		if len(production) == 0 {
			return nil
		}
		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		t.Run(filepath.ToSlash(relative), func(t *testing.T) {
			if len(production) == 1 && production[0] == "doc.go" {
				t.Fatal("documentation-only package creates a zero-behavior umbrella; keep this directory as a namespace")
			}
			packageName := filepath.Base(path)
			hasBoundaryDoc := false
			for _, name := range production {
				parsed, parseErr := parser.ParseFile(
					token.NewFileSet(), filepath.Join(path, name), nil, parser.ParseComments,
				)
				if parseErr != nil {
					t.Fatalf("parse %s: %v", name, parseErr)
				}
				if parsed.Name.Name != packageName {
					t.Errorf("%s declares package %s, want directory name %s", name, parsed.Name.Name, packageName)
				}
				if parsed.Doc != nil && strings.HasPrefix(parsed.Doc.Text(), "Package "+packageName+" ") {
					hasBoundaryDoc = true
				}
			}
			if !hasBoundaryDoc {
				t.Errorf("package has no boundary comment beginning with %q", "Package "+packageName)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan production package boundaries: %v", err)
	}
}
