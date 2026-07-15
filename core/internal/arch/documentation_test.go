package arch

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPublicPackagesHaveDocsAndRunnableExamples(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	packages := make([]string, 0, len(targetPublicPackages))
	for packagePath := range targetPublicPackages {
		packages = append(packages, packagePath)
	}
	sort.Strings(packages)

	for _, packagePath := range packages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDocumentation(t, filepath.Join(root, filepath.FromSlash(packagePath)))
		})
	}
}

func assertPackageDocumentation(t *testing.T, directory string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var productionFiles []*ast.File
	var testFiles []*ast.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(directory, entry.Name()), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			testFiles = append(testFiles, file)
		} else {
			productionFiles = append(productionFiles, file)
		}
	}

	docFiles := 0
	for _, file := range productionFiles {
		if file.Doc != nil {
			docFiles++
		}
	}
	if docFiles != 1 {
		t.Fatalf("package must have exactly one package comment, got %d", docFiles)
	}

	for _, example := range doc.Examples(testFiles...) {
		if example.Name == "" && example.Output != "" {
			return
		}
	}
	t.Fatal("package must have a package-level Example with checked Output")
}
