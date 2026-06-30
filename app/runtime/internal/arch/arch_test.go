// Package arch holds the module's architecture-fitness tests. It contains no
// production code — only tests that mechanically enforce structural invariants
// the compiler can't, so the architecture can't quietly rot during refactors.
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

// TestDependencyRule enforces Clean Architecture's Dependency Rule for the lyra
// module: source dependencies point INWARD, toward the domain. Outer rings
// (delivery / capability / infra adapters) may depend on inner rings; the
// reverse — or a sibling adapter reaching across the core — is forbidden. See
// doc/GREENFIELD_ARCHITECTURE.md.
//
// Rings (outer → inner):
//
//	delivery       internal/delivery/**         HTTP+SSE / inprocess transport, dispatch, protocol
//	adapter        internal/adapter/**          capability adapters (tools, live external capabilities)
//	orchestration  internal/kernel/**           use-case core (drives the agent loop; ACL over the agent SDK)
//	domain         internal/domain/**          bounded contexts: entities + repository ports + domain services
//	infra          internal/infra/**            sqlite / git / lsp / mcp / exec — driven adapters & frameworks
//	composition    internal/runtime,            the "main" component that wires everything; exempt as an importer
//	               internal/config, cmd/**
//
// Forbidden edges (an inner ring learning about an outer one, or an adapter
// reaching across the core):
//
//	infra         ↛ delivery, adapter, orchestration
//	domain        ↛ delivery, adapter
//	orchestration ↛ delivery, adapter
//	adapter       ↛ delivery
//
// Intentionally NOT forbidden (each is a correct inward / hexagonal edge —
// documented in GREENFIELD_ARCHITECTURE.md §5):
//
//	kernel            → domain/*    orchestration depends inward on domain
//	infra             → domain/*    adapter depends inward on domain entities + repo ports
//	domain/maintenance → kernel     maintenance is a driven adapter of the kernel's
//	                                 Compactor/Extractor PORTS; importing the port
//	                                 owner for its DTOs is the correct hexagonal direction
//	adapter          → kernel/*     capability adapters implement kernel-owned
//	                                 ports (tool resolver, MCP live control)
//	adapter          → infra/*      capability adapters wrap driven capabilities
//	delivery          → anything inward
func TestDependencyRule(t *testing.T) {
	const modulePath = "github.com/Tangerg/lynx/app/runtime"
	root := moduleRoot(t)
	fset := token.NewFileSet()

	violations := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		// Test files may import across rings (stubs, fixtures); only production
		// dependencies are constrained.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		from := layerOf(filepath.ToSlash(rel))
		if from == "" || from == ringComposition {
			return nil // unclassified or exempt importer
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			rest, ok := strings.CutPrefix(ip, modulePath+"/")
			if !ok {
				continue
			}
			to := layerOf(rest)
			if to != "" && forbidden(from, to) {
				violations++
				t.Errorf("dependency-rule violation: %s (%s) imports %s (%s)", rel, from, rest, to)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module: %v", walkErr)
	}
	if violations == 0 {
		t.Log("dependency rule holds: all cross-ring edges point inward")
	}
}

const (
	ringDelivery      = "delivery"
	ringAdapter       = "adapter"
	ringOrchestration = "orchestration"
	ringDomain        = "domain"
	ringInfra         = "infra"
	ringComposition   = "composition"
)

// layerOf classifies a module-relative package dir (e.g. "internal/infra/storage")
// into its ring, or "" when the path is outside the rings under test.
func layerOf(rel string) string {
	switch {
	case rel == "internal/runtime" || rel == "internal/config" || strings.HasPrefix(rel, "cmd/"):
		return ringComposition
	case strings.HasPrefix(rel, "internal/delivery/"):
		return ringDelivery
	case strings.HasPrefix(rel, "internal/adapter/"):
		return ringAdapter
	case rel == "internal/kernel" || strings.HasPrefix(rel, "internal/kernel/"):
		return ringOrchestration
	case strings.HasPrefix(rel, "internal/domain/"):
		return ringDomain
	case strings.HasPrefix(rel, "internal/infra/"):
		return ringInfra
	default:
		return ""
	}
}

// forbidden reports whether a package in ring "from" may NOT import one in "to".
func forbidden(from, to string) bool {
	switch from {
	case ringInfra:
		return to == ringDelivery || to == ringAdapter || to == ringOrchestration
	case ringDomain:
		return to == ringDelivery || to == ringAdapter || to == ringInfra
	case ringOrchestration:
		return to == ringDelivery || to == ringAdapter || to == ringInfra
	case ringAdapter:
		return to == ringDelivery
	default:
		return false
	}
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod (the lyra module root).
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}
