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
// reverse — or a sibling adapter reaching up into composition — is forbidden.
// See doc/EXECUTION_CENTERED_ARCHITECTURE.md.
//
// Rings (outer → inner):
//
//	delivery       internal/delivery/**         HTTP+SSE / inprocess transport, dispatch, protocol
//	adapter        internal/adapter/**          capability adapters, incl. adapter/agentexec (the
//	                                             agent-execution adapter driving the agent loop / ACL
//	                                             over the agent SDK) and infra-backed capability adapters
//	domain         internal/domain/**          bounded contexts: entities + repository ports + domain services
//	infra          internal/infra/**            sqlite / git / lsp / mcp / exec — driven adapters & frameworks
//	composition    internal/runtime/**,         the "main" component that wires everything; exempt as an importer
//	               internal/bootstrap/**,       (config load + assembly + host lifecycle live in internal/bootstrap)
//	               internal/config, cmd/**
//
// Forbidden edges (an inner ring learning about an outer one, or an adapter
// reaching up into composition):
//
//	infra   ↛ delivery, adapter
//	domain  ↛ delivery, adapter, infra
//	adapter ↛ delivery, composition
//
// Intentionally NOT forbidden (each is a correct inward / hexagonal edge —
// documented in EXECUTION_CENTERED_ARCHITECTURE.md §3 / §6):
//
//	adapter → domain/*    capability + agent-execution adapters depend inward on domain entities + ports
//	infra   → domain/*    driven adapters depend inward on domain entities + repo ports
//	adapter → adapter/*   sibling adapters compose (e.g. toolset implements agentexec's tool SPI)
//	adapter → infra/*     capability adapters wrap driven capabilities
//	delivery → anything inward
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

func TestDomainHooksStayPure(t *testing.T) {
	root := moduleRoot(t)
	forbidden := map[string]struct{}{
		"os":            {},
		"os/exec":       {},
		"path/filepath": {},
	}
	walkErr := filepath.WalkDir(filepath.Join(root, "internal", "domain", "hooks"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			if _, bad := forbidden[ip]; bad {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("domain hooks must not import %s: %s", ip, rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk domain hooks: %v", walkErr)
	}
}

const (
	ringDelivery    = "delivery"
	ringAdapter     = "adapter"
	ringDomain      = "domain"
	ringInfra       = "infra"
	ringComposition = "composition"
)

// layerOf classifies a module-relative package dir (e.g. "internal/infra/storage")
// into its ring, or "" when the path is outside the rings under test.
func layerOf(rel string) string {
	switch {
	case rel == "internal/runtime" || strings.HasPrefix(rel, "internal/runtime/") ||
		rel == "internal/bootstrap" || strings.HasPrefix(rel, "internal/bootstrap/") ||
		rel == "internal/config" || strings.HasPrefix(rel, "cmd/"):
		return ringComposition
	case rel == "internal/delivery" || strings.HasPrefix(rel, "internal/delivery/"):
		return ringDelivery
	case rel == "internal/adapter" || strings.HasPrefix(rel, "internal/adapter/"):
		return ringAdapter
	case rel == "internal/domain" || strings.HasPrefix(rel, "internal/domain/"):
		return ringDomain
	case rel == "internal/infra" || strings.HasPrefix(rel, "internal/infra/"):
		return ringInfra
	default:
		return ""
	}
}

// forbidden reports whether a package in ring "from" may NOT import one in "to".
func forbidden(from, to string) bool {
	switch from {
	case ringInfra:
		return to == ringDelivery || to == ringAdapter
	case ringDomain:
		return to == ringDelivery || to == ringAdapter || to == ringInfra
	case ringAdapter:
		// Adapters implement domain/application ports and wrap infra; they must
		// never reach up into the composition root (internal/runtime / config /
		// cmd) — that inversion would let assembly logic hide inside a capability
		// adapter (the startup-projection edge that prompted this guard).
		return to == ringDelivery || to == ringComposition
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
