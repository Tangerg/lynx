package conformance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// releaseBackends is the complete published backend set. Keep it explicit:
// adding a backend is a release decision and must include a conformance suite.
var releaseBackends = []string{
	"azureaisearch",
	"azurecosmos",
	"bedrockkb",
	"cassandra",
	"chroma",
	"clickhouse",
	"cockroachdb",
	"couchbase",
	"elasticsearch",
	"inmemory",
	"mariadb",
	"milvus",
	"mongodb",
	"neo4j",
	"opensearch",
	"oracle",
	"pgvector",
	"pinecone",
	"qdrant",
	"redis",
	"s3vectors",
	"supabase",
	"tidb",
	"typesense",
	"vectara",
	"vespa",
	"weaviate",
}

func TestReleaseBackendCoverage(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	discovered := discoverBackendPackages(t, root)
	if !slices.Equal(discovered, releaseBackends) {
		t.Fatalf("published backend set changed\nwant: %v\n got: %v\nupdate releaseBackends and add a conformance suite", releaseBackends, discovered)
	}

	for _, backend := range releaseBackends {
		backend := backend
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			assertConformanceSuite(t, filepath.Join(root, backend, "conformance_test.go"))
		})
	}
}

func discoverBackendPackages(t *testing.T, root string) []string {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	var backends []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "internal" || entry.Name() == "third_party" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, entry.Name(), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 0 {
			backends = append(backends, entry.Name())
		}
	}
	slices.Sort(backends)
	return backends
}

func assertConformanceSuite(t *testing.T, filename string) {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse conformance suite: %v", err)
	}

	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if path != "github.com/Tangerg/lynx/vectorstores/internal/conformance" {
			continue
		}
		alias := "conformance"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = struct{}{}
	}

	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Run" {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok {
			_, found = aliases[ident.Name]
		}
		return !found
	})
	if !found {
		t.Fatalf("%s must import internal/conformance and call conformance.Run", filename)
	}
}
