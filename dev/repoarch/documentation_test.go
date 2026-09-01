package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

var documentationOnlyModuleRoots = []string{"core", "examples", "otel", "tools"}

func TestWorkspaceModulesKeepDocumentationEntryPoints(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)
	for _, modulePath := range slices.Sorted(maps.Keys(modules)) {
		module := modules[modulePath]
		t.Run(module.dir, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(root, filepath.FromSlash(module.dir), "doc.go")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("module %s is missing doc.go: %v", module.path, err)
			}
			if !info.Mode().IsRegular() || info.Size() == 0 {
				t.Fatalf("module %s has no readable content in doc.go", module.path)
			}
			assertPackageOverview(t, path)
			for _, retired := range []string{"README.md", "ARCHITECTURE.md"} {
				retiredPath := filepath.Join(root, filepath.FromSlash(module.dir), retired)
				if _, err := os.Stat(retiredPath); !os.IsNotExist(err) {
					t.Errorf("module %s keeps retired parallel documentation %s", module.path, retired)
				}
			}
		})
	}
}

// TestNamespaceDirectoriesKeepOneDocumentationEntry gates the directories that
// group modules without being modules themselves. They cannot carry a package
// comment, so README.md is their one entry point; a second parallel file would
// reintroduce exactly the split
// TestWorkspaceModulesKeepDocumentationEntryPoints removes from every module.
func TestNamespaceDirectoriesKeepOneDocumentationEntry(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	namespaces := discoverNamespaceDirectories(t, root)
	if len(namespaces) == 0 {
		t.Fatal("discovered no namespace directories; the gate would pass vacuously")
	}
	for _, namespace := range namespaces {
		t.Run(namespace, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, filepath.FromSlash(namespace))
			info, err := os.Stat(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatalf("namespace %s is missing README.md: %v", namespace, err)
			}
			if !info.Mode().IsRegular() || info.Size() == 0 {
				t.Fatalf("namespace %s has no readable content in README.md", namespace)
			}
			for _, retired := range []string{"ARCHITECTURE.md", "doc.go"} {
				if _, err := os.Stat(filepath.Join(dir, retired)); !os.IsNotExist(err) {
					t.Errorf("namespace %s keeps a second documentation entry %s", namespace, retired)
				}
			}
		})
	}
}

// discoverNamespaceDirectories returns the top-level directories that group
// workspace modules but declare no module of their own. Deriving them from the
// workspace rather than listing them keeps a newly added family from silently
// escaping the gate.
func discoverNamespaceDirectories(t *testing.T, root string) []string {
	t.Helper()
	namespaces := make(map[string]struct{})
	for _, module := range discoverModules(t, root) {
		prefix, _, nested := strings.Cut(filepath.ToSlash(module.dir), "/")
		if !nested {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, prefix, "go.mod")); os.IsNotExist(err) {
			namespaces[prefix] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(namespaces))
}

func TestRepositoryGuidanceHasOneCanonicalSource(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	claudePath := filepath.Join(root, "CLAUDE.md")
	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(claude), "@./AGENTS.md") != 1 {
		t.Error("CLAUDE.md must point to the canonical AGENTS.md exactly once")
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipRepositoryDir(filepath.ToSlash(relativePath), entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if relativePath == "AGENTS.md" || relativePath == "CLAUDE.md" {
			return nil
		}
		if entry.Name() == "AGENTS.md" || entry.Name() == "CLAUDE.md" {
			t.Errorf("%s duplicates repository guidance; put package contracts in GoDoc", filepath.ToSlash(relativePath))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDocumentationOnlyModuleRootsStayDocumentationOnly keeps an overview from
// becoming the second public entry for capabilities owned by child packages.
func TestDocumentationOnlyModuleRootsStayDocumentationOnly(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relative := range documentationOnlyModuleRoots {
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			directory := filepath.Join(root, relative)
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				if entry.Name() != "doc.go" {
					t.Errorf("documentation-only module root %s contains production file %s", relative, entry.Name())
				}
			}

			path := filepath.Join(directory, "doc.go")
			file := assertPackageOverview(t, path)
			if len(file.Decls) != 0 {
				t.Errorf("%s/doc.go declares production API or implementation", relative)
			}
		})
	}
}

func assertPackageOverview(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if file.Doc == nil || !strings.HasPrefix(strings.TrimSpace(file.Doc.Text()), "Package "+file.Name.Name) {
		t.Errorf("%s has no package overview", path)
	}
	return file
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
