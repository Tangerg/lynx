// Package arch holds the module's architecture-fitness tests. It contains no
// production code — only tests that mechanically enforce structural invariants
// the compiler can't, so the architecture can't quietly rot during refactors.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDependencyRule enforces Clean Architecture's Dependency Rule for the lyra
// module: source dependencies point INWARD, toward the domain. Outer rings may
// depend on inner rings; the reverse — or a driven/adapter ring reaching up into
// the composition root — is forbidden. See doc/EXECUTION_CENTERED_ARCHITECTURE.md.
//
// Rings (outer → inner):
//
//	composition    internal/bootstrap/**,        the "main" component: config load, assembly, host
//	               internal/config, cmd/**       lifecycle. Wires every ring, so it imports anything —
//	                                             but nothing imports IT.
//	delivery       internal/delivery/**          HTTP+SSE / inprocess transport, dispatch, protocol
//	adapter        internal/adapter/**           capability adapters, incl. adapter/agentexec (the
//	                                              agent-execution adapter over the agent SDK)
//	application    internal/application/**        use-case coordinators (runs / sessions / capabilities /
//	                                              workspace / schedules) — engine- and wire-neutral
//	infra          internal/infra/**             sqlite / git / lsp / mcp / exec — driven adapters & frameworks
//	domain         internal/domain/**            bounded contexts: entities + repository ports + domain services
//
// Forbidden edges (an inner ring learning about an outer one, a driven ring
// reaching sideways/up, or anything importing the composition root):
//
//	domain      ↛ application, adapter, infra, delivery, composition
//	application ↛ adapter, infra, delivery, composition   (§19: application imports no SDK/SQLite/protocol)
//	infra       ↛ application, adapter, delivery, composition   (driven adapter: imports only domain)
//	adapter     ↛ delivery, composition
//	delivery    ↛ infra, composition   (drives ports/adapters, never raw storage)
//
// Intentionally allowed inward / hexagonal edges (EXECUTION_CENTERED_ARCHITECTURE.md §3 / §6):
//
//	application → domain          coordinators depend on entities + consumer-side ports
//	adapter → domain, application capability + agent-execution adapters implement application/domain ports
//	adapter → adapter, infra      sibling adapters compose; capability adapters wrap driven capabilities
//	infra   → domain              driven adapters implement domain repo ports
//	delivery → domain, application, adapter
//	composition → anything        the root wires every ring
//
// The dependency edges above are the backbone; §16's remaining rules get their
// own dedicated tests below so each invariant is independently guarded, not left
// to a transitive consequence: rule 2/domain-purity (incl. the internal/component
// concurrency-primitive edge the ring rule leaves unclassified), rule 4/5/8, rule
// 9 (application ↛ protocol, TestApplicationEventFreeOfProtocol) and rule 10
// (protocol ↛ domain/application, TestProtocolStaysWireOnly). Rule 11's domain
// DAG is the Go compiler's (import cycles don't build).
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
		if from == "" {
			return nil // unclassified importer (e.g. a module-root helper)
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

// TestDomainHooksStayPure keeps the hooks bounded context free of filesystem +
// process I/O: hooks is a pure policy domain (precedence / merge / trust rules),
// and its I/O belongs to the composition-side subprocess adapter.
func TestDomainHooksStayPure(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "domain", "hooks"),
		[]string{"os", "os/exec", "path/filepath"})
}

// TestDomainStaysFrameworkFree keeps every bounded context free of frameworks +
// heavy runtime coupling (§19 "domain 不引入 I/O/framework"): no filesystem or
// process I/O, network, database driver, or external SDK/storage library
// (including the reusable chathistory adapter contract).
// Pure path-string composition via path/filepath remains allowed. The single
// agent-SDK edge (accounting reads core.ModelCall token counts, a value
// type) is a deliberate, documented exception and stays allowed.
func TestDomainStaysFrameworkFree(t *testing.T) {
	root := moduleRoot(t)
	// componentPkg is banned here (not in frameworkImports) because application
	// legitimately imports internal/component/taskgroup — only the inner domain
	// ring must stay free of it, and layerOf leaves component unclassified so the
	// ring rule alone would miss a domain → component edge.
	forbidExternalImports(t, filepath.Join(root, "internal", "domain"),
		append([]string{componentPkg}, frameworkImports...))
}

// TestApplicationStaysFrameworkFree enforces §19's headline application-purity
// clause directly for EXTERNAL dependencies (the ring rule already forbids the
// internal SDK/SQLite/protocol edges): a use-case coordinator imports no agent
// SDK, concrete chat client, SQLite driver, Git, MCP, or LSP library. Its only
// cross-module values are the neutral core chat/media contracts.
func TestApplicationStaysFrameworkFree(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "application"),
		append([]string{
			"github.com/Tangerg/lynx/agent",
			"github.com/Tangerg/lynx/chatclient",
		}, frameworkImports...))
}

// TestExecutionDomainStaysPure enforces §16 rule 2: the core execution context
// (domain/execution + its sub-contexts) is the innermost, most-protected code —
// it must not touch the filesystem, a SQL driver, HTTP, OTel, the agent SDK, or a
// concurrency/wiring primitive (internal/component/*). (The accounting sub-context
// maps the SDK's token counts at the agentexec boundary, so it holds only the
// neutral core chat model, never agent/*.) The component ban is listed explicitly
// because layerOf leaves internal/component unclassified — the ring rule would not
// otherwise catch a domain → component/taskgroup edge.
func TestExecutionDomainStaysPure(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "domain", "execution"),
		[]string{"os", "database/sql", "net", "net/http", "go.opentelemetry.io", "github.com/Tangerg/lynx/agent", componentPkg})
}

// TestDeliveryStaysAdapterOnly enforces §16 rule 4: delivery drives ports, so it
// imports no agent SDK / SQLite driver / Git / MCP / LSP library directly (the
// ring rule already forbids the internal infra edge; this covers the EXTERNAL
// libraries). net/http is NOT banned — it is delivery's own transport.
func TestDeliveryStaysAdapterOnly(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery"), externalSDKs)
}

// TestDeliveryDoesNotControlAgentTurns keeps complete Run commands behind the
// application/runs use-case surface. Delivery may decode and present wire data,
// but it must not plan, rebuild, assert, or steer concrete agent turn handles.
func TestDeliveryDoesNotControlAgentTurns(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery"),
		[]string{"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"})
}

// TestUseCasesDoNotDependOnConcreteAgentEngine keeps the agent runtime behind
// bootstrap and the turn dispatcher. Application owns consumer-side ports and
// delivery invokes use cases; neither ring may regain a dependency on the
// concrete agentexec Engine or one of its implementation subpackages.
func TestUseCasesDoNotDependOnConcreteAgentEngine(t *testing.T) {
	const agentExecPkg = "github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "application"), []string{agentExecPkg})
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery"), []string{agentExecPkg})
}

// TestAgentExecDelegatesManagedExecution locks the Framework/Host ownership
// boundary. The agent adapter may supply product prompts, pricing, observers,
// tools, and responses, but it must not rebuild the framework's ToolLoop,
// decode ProcessSnapshot continuation payloads, or record framework usage
// directly. Those concerns belong to agent/runtime's managed interaction and
// persistence coordinator.
func TestAgentExecDelegatesManagedExecution(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "adapter", "agentexec")
	forbidExternalImports(t, dir, []string{"github.com/Tangerg/lynx/agent/toolloop"})

	forbiddenSelectors := map[string]string{
		"core.ProcessSnapshot": "Host adapters must treat process snapshots as framework-owned persistence",
		"toolloop.NewRunner":   "managed interaction owns the ToolLoop runner",
		"pc.RecordModelCall":   "managed interaction owns framework usage recording",
		"proc.RecordModelCall": "managed interaction owns framework usage recording",
	}
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := exprString(selector.X) + "." + selector.Sel.Name
			if reason, forbidden := forbiddenSelectors[name]; forbidden {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: %s uses %s", rel, reason, name)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk agentexec: %v", walkErr)
	}
}

// TestCapabilityAdaptersDoNotImportTransportSDKs keeps MCP/A2A protocol
// libraries behind internal/infra. Tool assembly consumes the infrastructure
// adapters through local configuration and the narrow tools.Tool capability;
// it must not construct or expose transport-library types itself.
func TestCapabilityAdaptersDoNotImportTransportSDKs(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "adapter", "toolset"), []string{
		"github.com/Tangerg/lynx/a2a",
		"github.com/Tangerg/lynx/mcp",
		"github.com/a2aproject/a2a-go",
		"github.com/modelcontextprotocol/go-sdk",
		"github.com/mark3labs/mcp-go",
	})
}

// TestBootstrapExposesNoBusinessMethod enforces §16 rule 8: the composition root
// assembles and closes — it must not become a business facade. Assembly / config
// / seed FUNCTIONS are fine (they return the Stack); the guard is that exported
// TYPES in bootstrap (Host / Stack) carry only lifecycle methods, so a business
// method like `Host.RollbackSession` — which delivery could call instead of a
// coordinator — can't sneak in.
func TestBootstrapExposesNoBusinessMethod(t *testing.T) {
	root := moduleRoot(t)
	allowedMethods := map[string]struct{}{"Close": {}}
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(filepath.Join(root, "internal", "bootstrap"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || !receiverIsExported(fn.Recv) {
				continue // plain funcs (assembly) + methods on unexported types are fine
			}
			if _, ok := allowedMethods[fn.Name.Name]; ok {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: exported method %s on an exported bootstrap type — bootstrap may only assemble/close (§16 rule 8)", rel, fn.Name.Name)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk bootstrap: %v", walkErr)
	}
}

// TestHostOwnsShutdownGraph enforces the B9 resource boundary without relying
// on source text: Host owns one shared lifetime, and that lifetime owns every
// process-level shutdown stage plus tool/process resources. Engine must not
// regain resource ownership.
func TestHostOwnsShutdownGraph(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "bootstrap")
	structs := map[string]*ast.StructType{}
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				named, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if value, ok := named.Type.(*ast.StructType); ok {
					structs[named.Name.Name] = value
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk bootstrap: %v", walkErr)
	}

	host := structFieldNames(structs["Host"])
	if _, ok := host["lifetime"]; !ok {
		t.Fatal("bootstrap.Host must own the shared shutdown lifetime")
	}
	for _, forbidden := range []string{"engine", "toolClosers", "resources"} {
		if _, ok := host[forbidden]; ok {
			t.Errorf("bootstrap.Host must not copy %s outside its shared lifetime", forbidden)
		}
	}

	lifetime := structFieldNames(structs["hostLifetime"])
	for _, required := range []string{
		"integrations",
		"codebase",
		"coordinator",
		"dispatcher",
		"effectsTasks",
		"toolClosers",
		"resources",
	} {
		if _, ok := lifetime[required]; !ok {
			t.Errorf("bootstrap.hostLifetime must own %s", required)
		}
	}
	if _, ok := lifetime["engine"]; ok {
		t.Error("bootstrap.hostLifetime owns engine; Agent execution must not be a resource closer")
	}
}

// TestDeliveryHoldsNoRunLifecycleState enforces §16 rule 5: the delivery Server
// (the protocol handler) drives the run coordinator as a use-case surface, but
// must not itself HOLD the run registry, a cancel func, a task group, or a
// checkpoint store — the run-lifecycle ownership §20 moved to the application/Host.
// Scoped to delivery/server: the transport packages legitimately own their own
// call-lifecycle task groups. Two forms: (a) the task group is import-forbidden
// outright (a field would need the import; this also catches a held cancel-func
// group); (b) a struct-field AST walk forbids a held checkpoint store or run
// registry, whose packages the Server imports for other reasons (adapter/
// workspace's GitAvailable probe; application/runs' Coordinator + Event).
func TestDeliveryHoldsNoRunLifecycleState(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "delivery", "server")
	forbidExternalImports(t, dir, []string{"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"})

	// taskgroup.Group is also import-forbidden above; context.CancelFunc and
	// runs.Registry cover the rule's "cancel func" + "run registry" clauses so a
	// hand-rolled live-run map's cancel handles can't be parked on the Server.
	forbiddenFields := []string{"taskgroup.Group", "workspace.Checkpoints", "runs.Registry", "context.CancelFunc"}
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				ts := exprString(field.Type)
				for _, bad := range forbiddenFields {
					if strings.Contains(ts, bad) {
						rel, _ := filepath.Rel(root, path)
						t.Errorf("%s: delivery struct holds %s — run-lifecycle state belongs to the coordinator/Host (§16 rule 5)", rel, bad)
					}
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk delivery: %v", walkErr)
	}
}

// exprString renders a field's type expression to a "pkg.Type" / "*pkg.Type[…]"
// string for substring matching. Unhandled shapes render "", which matches no
// rule (the checks are allow-by-default).
func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.IndexExpr:
		return exprString(t.X) + "[" + exprString(t.Index) + "]"
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	default:
		return ""
	}
}

func structFieldNames(value *ast.StructType) map[string]struct{} {
	out := map[string]struct{}{}
	if value == nil || value.Fields == nil {
		return out
	}
	for _, field := range value.Fields.List {
		for _, name := range field.Names {
			out[name.Name] = struct{}{}
		}
	}
	return out
}

// TestApplicationEventFreeOfProtocol enforces §16 rule 9: application (its Events,
// commands, ports) never references a protocol/wire type. The ring rule already
// forbids application → delivery; this is the dedicated, explicit guard so the
// invariant survives even if a protocol type were ever mislocated outside delivery.
func TestApplicationEventFreeOfProtocol(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "application"), []string{protocolPkg})
}

// TestProtocolStaysWireOnly enforces §16 rule 10: protocol types don't enter
// domain/application — the wire package itself must import neither ring, so wire
// shapes never become a business dependency. (Delivery as a whole MAY import
// domain/application to drive them; this constrains only the protocol subpackage.)
func TestProtocolStaysWireOnly(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery", "protocol"),
		[]string{
			"github.com/Tangerg/lynx/app/runtime/internal/domain",
			"github.com/Tangerg/lynx/app/runtime/internal/application",
		})
}

// TestCanonicalExecutionRecordsStayTyped prevents the old persistence design
// from returning: transcript and interrupt records may be serialized by an
// adapter, but their domain shape cannot contain an opaque Blob/Payload/JSON
// field or json.RawMessage.
func TestCanonicalExecutionRecordsStayTyped(t *testing.T) {
	root := moduleRoot(t)
	dirs := []string{
		filepath.Join(root, "internal", "domain", "execution", "transcript"),
		filepath.Join(root, "internal", "domain", "execution", "interrupts"),
	}
	for _, dir := range dirs {
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "encoding/json" {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: canonical execution records must not depend on JSON", rel)
				}
			}
			ast.Inspect(f, func(node ast.Node) bool {
				field, ok := node.(*ast.Field)
				if !ok {
					return true
				}
				for _, name := range field.Names {
					switch name.Name {
					case "Blob", "Payload", "JSON":
						rel, _ := filepath.Rel(root, path)
						t.Errorf("%s: canonical execution field %s reintroduces an opaque persistence payload", rel, name.Name)
					}
				}
				return true
			})
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
}

// TestRunReductionHasNoOuterProjectionSeam locks in the ownership cutover:
// application/runs reduces EngineEvent into canonical RunEvent itself, and
// delivery cannot recreate the former stateful Projector/translator or derive
// durable side effects from protocol events.
func TestRunReductionHasNoOuterProjectionSeam(t *testing.T) {
	root := moduleRoot(t)
	banned := map[string]struct{}{
		"Projector": {}, "Projection": {}, "ProjectedEvent": {}, "SegmentView": {},
		"sideEffectEvent": {}, "newTranslator": {}, "translator": {},
	}
	dirs := []string{
		filepath.Join(root, "internal", "application", "runs"),
		filepath.Join(root, "internal", "delivery", "server"),
	}
	for _, dir := range dirs {
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				switch decl := decl.(type) {
				case *ast.FuncDecl:
					if _, found := banned[decl.Name.Name]; found {
						rel, _ := filepath.Rel(root, path)
						t.Errorf("%s: obsolete run projection seam %s", rel, decl.Name.Name)
					}
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						if typ, ok := spec.(*ast.TypeSpec); ok {
							if _, found := banned[typ.Name.Name]; found {
								rel, _ := filepath.Rel(root, path)
								t.Errorf("%s: obsolete run projection type %s", rel, typ.Name.Name)
							}
						}
					}
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
}

// receiverIsExported reports whether a method's receiver is a (pointer to an)
// exported named type.
func receiverIsExported(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	typ := recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	id, ok := typ.(*ast.Ident)
	return ok && id.IsExported()
}

// componentPkg is the neutral concurrency/wiring primitive package
// (taskgroup / filechanges / mcpstatus). layerOf leaves it unclassified so the
// ring rule doesn't check edges into it; the domain rings ban it explicitly
// (application/delivery/composition may import it). Prefix-matched.
const componentPkg = "github.com/Tangerg/lynx/app/runtime/internal/component"

// protocolPkg is the wire-type package; it must stay pure wire (no domain /
// application import) so protocol types never leak inward (§16 rule 10).
const protocolPkg = "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// externalSDKs are the external agent-SDK / driver / framework libraries the
// inner + delivery rings must never import directly (the internal infra edges are
// covered by the ring rule). Prefix-matched.
var externalSDKs = []string{
	"github.com/Tangerg/lynx/agent",
	"modernc.org/sqlite",
	"github.com/go-git",
	"github.com/mark3labs",
	"github.com/sourcegraph",
}

// frameworkImports are the framework / driver / SDK packages an inner ring must
// never import. Prefix-matched, so e.g. "modernc.org/sqlite" catches the driver
// and its sub-packages.
var frameworkImports = []string{
	"os",
	"io/fs",
	"net",
	"net/http",
	"database/sql",
	"modernc.org/sqlite",
	"github.com/go-git",
	"github.com/mark3labs",
	"github.com/sourcegraph",
	"github.com/Tangerg/lynx/chathistory",
}

// forbidExternalImports fails the test for any production file under dir whose
// import path equals or (for framework roots) is prefixed by a forbidden entry.
// Exact std-lib names ("net") match the package itself and its sub-packages
// ("net/http") without matching unrelated names.
func forbidExternalImports(t *testing.T, dir string, banned []string) {
	t.Helper()
	root := moduleRoot(t)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
			for _, bad := range banned {
				if ip == bad || strings.HasPrefix(ip, bad+"/") {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s must not import %q", rel, ip)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
}

const (
	ringComposition = "composition"
	ringDelivery    = "delivery"
	ringAdapter     = "adapter"
	ringApplication = "application"
	ringInfra       = "infra"
	ringDomain      = "domain"
)

// layerOf classifies a module-relative package dir (e.g. "internal/infra/storage")
// into its ring, or "" when the path is outside the rings under test.
func layerOf(rel string) string {
	switch {
	case rel == "internal/bootstrap" || strings.HasPrefix(rel, "internal/bootstrap/") ||
		rel == "internal/config" || strings.HasPrefix(rel, "cmd/"):
		return ringComposition
	case rel == "internal/delivery" || strings.HasPrefix(rel, "internal/delivery/"):
		return ringDelivery
	case rel == "internal/adapter" || strings.HasPrefix(rel, "internal/adapter/"):
		return ringAdapter
	case rel == "internal/application" || strings.HasPrefix(rel, "internal/application/"):
		return ringApplication
	case rel == "internal/infra" || strings.HasPrefix(rel, "internal/infra/"):
		return ringInfra
	case rel == "internal/domain" || strings.HasPrefix(rel, "internal/domain/"):
		return ringDomain
	default:
		return ""
	}
}

// forbidden reports whether a package in ring "from" may NOT import one in "to".
// The composition root (runtime facade / bootstrap / config / cmd) wires every
// ring, so it forbids nothing as an importer — but it is a forbidden TARGET for
// every other ring, so assembly logic can never be pulled back into a business
// ring (there is no blanket skip: composition is a normal ring here that happens
// to import freely, while nothing imports it).
func forbidden(from, to string) bool {
	switch from {
	case ringDomain:
		return to != ringDomain
	case ringApplication:
		return to != ringDomain && to != ringApplication
	case ringInfra:
		// A driven adapter implements domain ports; it must never reach out to
		// application, sibling adapters, delivery, or the composition root.
		return to != ringDomain && to != ringInfra
	case ringAdapter:
		// Adapters implement domain/application ports and wrap infra; they must
		// never reach up into delivery or the composition root (the latter would
		// let assembly logic hide inside a capability adapter).
		return to == ringDelivery || to == ringComposition
	case ringDelivery:
		// Delivery drives coordinators + adapters through ports; it never touches
		// raw storage (infra) or imports the root that wires it.
		return to == ringInfra || to == ringComposition
	default: // composition imports anything inward
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
