package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

var documentedCapabilityModules = []string{
	"a2a",
	"agent",
	"etl",
	"eval",
	"mcp",
	"otel",
	"rag",
	"skills",
	"tools",
}

// TestCapabilityModulesDocumentTheirPublicSurface keeps documentation
// discovery coupled to package discovery: new public package directories fail
// until their ownership is stated, and every capability module must retain an
// executable ordinary-path example.
func TestCapabilityModulesDocumentTheirPublicSurface(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relativeModule := range documentedCapabilityModules {
		relativeModule := relativeModule
		t.Run(relativeModule, func(t *testing.T) {
			t.Parallel()
			moduleRoot := filepath.Join(root, filepath.FromSlash(relativeModule))
			packages, checkedExamples := inspectModuleDocumentation(t, moduleRoot)
			if len(packages) == 0 {
				t.Fatalf("capability module %s has no public packages", relativeModule)
			}
			for directory, packageDocumentation := range packages {
				if !packageDocumentation {
					t.Errorf("public package %s has no Package comment in production code", filepath.ToSlash(directory))
				}
			}
			if checkedExamples == 0 {
				t.Errorf("capability module %s has no checked Go example", relativeModule)
			}
		})
	}
}

func inspectModuleDocumentation(t *testing.T, moduleRoot string) (map[string]bool, int) {
	t.Helper()
	packages := make(map[string]bool)
	checkedExamples := 0
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleRoot && excludedDocumentationDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			checkedExamples += countCheckedExamples(file)
			return nil
		}
		if file.Name.Name == "main" {
			return nil
		}
		directory := filepath.Dir(path)
		if _, found := packages[directory]; !found {
			packages[directory] = false
		}
		if file.Doc != nil && strings.HasPrefix(strings.TrimSpace(file.Doc.Text()), "Package "+file.Name.Name) {
			packages[directory] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return packages, checkedExamples
}

func excludedDocumentationDirectory(name string) bool {
	return name == "examples" || name == "internal" || name == "testdata" || strings.HasPrefix(name, ".")
}

func countCheckedExamples(file *ast.File) int {
	count := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || !strings.HasPrefix(function.Name.Name, "Example") {
			continue
		}
		for _, comment := range file.Comments {
			if comment.Pos() < function.Body.Pos() || comment.End() > function.Body.End() {
				continue
			}
			text := strings.TrimSpace(comment.Text())
			if strings.HasPrefix(text, "Output:") || strings.HasPrefix(text, "Unordered output:") {
				count++
				break
			}
		}
	}
	return count
}
