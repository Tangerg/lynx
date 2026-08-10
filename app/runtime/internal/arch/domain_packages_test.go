package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDomainPackagesDeclareAndVerifyTheirBoundaries makes every bounded context
// explicit. A small value package is welcome, but an undocumented or entirely
// untested directory cannot silently become a field-container dumping ground.
func TestDomainPackagesDeclareAndVerifyTheirBoundaries(t *testing.T) {
	domainRoot := filepath.Join(moduleRoot(t), "internal", "domain")
	entries, err := os.ReadDir(domainRoot)
	if err != nil {
		t.Fatalf("read Domain packages: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			assertReviewedDomainPackage(t, filepath.Join(domainRoot, entry.Name()), entry.Name())
		})
	}
}

func assertReviewedDomainPackage(t *testing.T, dir, packageName string) {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	hasTest := false
	hasPackageDoc := false
	for _, file := range files {
		if file.IsDir() {
			t.Errorf("nested directory %s splits one Domain boundary", file.Name())
			continue
		}
		if !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(file.Name(), "_test.go") {
			hasTest = true
			continue
		}
		parsed, parseErr := parser.ParseFile(
			token.NewFileSet(), filepath.Join(dir, file.Name()), nil, parser.ParseComments,
		)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", file.Name(), parseErr)
		}
		if parsed.Name.Name != packageName {
			t.Errorf("%s declares package %s, want directory name %s", file.Name(), parsed.Name.Name, packageName)
		}
		if parsed.Doc != nil && strings.HasPrefix(parsed.Doc.Text(), "Package "+packageName+" ") {
			hasPackageDoc = true
		}
	}
	if !hasPackageDoc {
		t.Errorf("package %s has no boundary comment beginning with %q", packageName, "Package "+packageName)
	}
	if !hasTest {
		t.Errorf("package %s has no direct Domain test", packageName)
	}
}
