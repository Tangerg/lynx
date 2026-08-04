// Package arch holds the module's architecture-fitness tests. It contains no
// production code — only tests that mechanically enforce structural invariants
// the compiler can't, so the architecture can't quietly rot during refactors.
package arch

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appcontract "github.com/Tangerg/lynx/app/runtime/internal/application/contract"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	deliveryserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
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

// TestRemovedProducerPortsDoNotReturnToDomain prevents the producer-owned
// interfaces removed during the ownership cleanup from quietly returning.
// Their method sets belong to their application or adapter consumers; Domain
// retains values, invariants, and only ports consumed by a Domain service.
func TestRemovedProducerPortsDoNotReturnToDomain(t *testing.T) {
	root := moduleRoot(t)
	for path, forbiddenNames := range map[string]map[string]struct{}{
		filepath.Join(root, "internal", "domain", "agentmemory"):   {"Store": {}},
		filepath.Join(root, "internal", "domain", "approval"):      {"Policy": {}},
		filepath.Join(root, "internal", "domain", "codebaseindex"): {"Index": {}},
		filepath.Join(root, "internal", "domain", "feedback"):      {"Store": {}},
		filepath.Join(root, "internal", "domain", "goal"):          {"Store": {}},
		filepath.Join(root, "internal", "domain", "knowledge"):     {"Store": {}},
		filepath.Join(root, "internal", "domain", "mcpserver"):     {"Registry": {}},
		filepath.Join(root, "internal", "domain", "schedule"):      {"Registry": {}},
		filepath.Join(root, "internal", "domain", "plan"):          {"Store": {}},
		filepath.Join(root, "internal", "domain", "tool"):          {"Catalog": {}, "Invoker": {}, "Registry": {}},
	} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			filePath := filepath.Join(path, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", filePath, err)
			}
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, spec := range general.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					if _, forbidden := forbiddenNames[typeSpec.Name.Name]; forbidden {
						t.Errorf("%s declares removed producer-owned domain interface %s", filePath, typeSpec.Name.Name)
					}
				}
			}
		}
	}
}

// TestTransparentAliasesStayAtTheTransportBoundary prevents one ring from
// pretending to own another ring's type through a transparent alias. The only
// deliberate aliases are the transport package's JSON-RPC vocabulary: they
// make the external codec types the shared wire boundary without duplicating
// the protocol implementation or leaking its import across every transport.
func TestTransparentAliasesStayAtTheTransportBoundary(t *testing.T) {
	root := moduleRoot(t)
	allowed := map[string]struct{}{
		filepath.Join("delivery", "transport", "transport.go:Message"):  {},
		filepath.Join("delivery", "transport", "transport.go:Request"):  {},
		filepath.Join("delivery", "transport", "transport.go:Response"): {},
		filepath.Join("delivery", "transport", "transport.go:ID"):       {},
		filepath.Join("delivery", "transport", "transport.go:Error"):    {},
	}
	internal := filepath.Join(root, "internal")
	err := filepath.WalkDir(internal, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(internal, path)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if !typeSpec.Assign.IsValid() {
					continue
				}
				key := rel + ":" + typeSpec.Name.Name
				if _, ok := allowed[key]; !ok {
					t.Errorf("%s declares transparent alias %s; use the owning type directly or define a semantic boundary type", rel, typeSpec.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan transparent aliases: %v", err)
	}
}

// TestRemovedStutteringVocabularyDoesNotReturn records the package-qualified
// names retired by the runtime vocabulary cleanup. Each replacement names the
// role once (turn.Handle, dispatch.Router, sessions.Activity, config.Settings)
// or corrects the concept (the turn Controller is not the JSON-RPC Router).
// Exact names keep this guard semantic instead of applying a brittle generic
// prefix rule to wire types such as protocol.ProtocolRange.
func TestRemovedStutteringVocabularyDoesNotReturn(t *testing.T) {
	root := moduleRoot(t)
	reason := "use the package's canonical, non-stuttering vocabulary"
	for path, names := range map[string][]string{
		filepath.Join(root, "internal", "adapter", "agentexec", "turn"): {
			"TurnHandle", "memoryDispatcher", "ErrDispatcherClosed",
		},
		filepath.Join(root, "internal", "adapter", "mcpconnection"): {"Connections"},
		filepath.Join(root, "internal", "application", "models"):    {"ModelDetails"},
		filepath.Join(root, "internal", "application", "sessions"): {
			"SessionStore", "SessionForgetter", "SessionAdmissions", "SessionAdmission",
			"SessionState", "SessionView",
		},
		filepath.Join(root, "internal", "application", "workspace"): {
			"ResolvedWorkspace", "WorkspaceSummary", "WorkspaceCatalog",
		},
		filepath.Join(root, "internal", "config"): {
			"Config", "ServerConfig", "OnlineConfig", "MCPServerConfig", "LSPServerConfig", "A2AAgentConfig",
		},
		filepath.Join(root, "internal", "delivery", "dispatch"): {"Dispatcher"},
	} {
		banned := make(map[string]string, len(names))
		for _, name := range names {
			banned[name] = reason
		}
		forbidTopLevelNames(t, path, banned)
	}
}

// TestRuntimeResponsibilityFilesStayFocused keeps the cleanup from collapsing
// back into catch-all files. These are not arbitrary size limits: each file is
// named for one lifecycle responsibility, and the forbidden declarations are
// independently meaningful use cases or process-ownership phases.
func TestRuntimeResponsibilityFilesStayFocused(t *testing.T) {
	root := moduleRoot(t)
	runs := filepath.Join(root, "internal", "application", "runs")
	for path, banned := range map[string]map[string]string{
		filepath.Join(runs, "opening.go"): {
			"Resume": "resume belongs to resume.go", "Cancel": "cancellation belongs to cancellation.go", "Steer": "steering belongs to steering.go",
		},
		filepath.Join(runs, "resume.go"): {
			"Start": "fresh opening belongs to opening.go", "Cancel": "cancellation belongs to cancellation.go", "Steer": "steering belongs to steering.go",
		},
		filepath.Join(runs, "cancellation.go"): {
			"Start": "fresh opening belongs to opening.go", "Resume": "resume belongs to resume.go", "Steer": "steering belongs to steering.go",
		},
		filepath.Join(runs, "steering.go"): {
			"Start": "fresh opening belongs to opening.go", "Resume": "resume belongs to resume.go", "Cancel": "cancellation belongs to cancellation.go",
		},
		filepath.Join(root, "internal", "bootstrap", "assemble.go"): {
			"Host": "process lifetime belongs to host.go", "hostLifetime": "process lifetime belongs to host.go",
			"RecoverStartup": "process startup lifetime belongs to host.go", "closeHostLifetime": "process lifetime belongs to host.go",
			"shutdownResources": "resource close mechanics belong to resources.go", "closePendingResources": "resource close mechanics belong to resources.go",
		},
	} {
		forbidTopLevelNames(t, path, banned)
	}
	if _, err := os.Stat(filepath.Join(runs, "usecases.go")); err == nil {
		t.Error("application/runs/usecases.go returned; keep independently named use cases in their focused files")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect removed runs/usecases.go: %v", err)
	}
}

// TestRemovedRedundantEntrypointsDoNotReturn keeps one callable surface per
// capability. Exported methods escape dead-code analysis, so the architecture
// suite records the two removed APIs explicitly.
func TestRemovedRedundantEntrypointsDoNotReturn(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "application", "sessions"), map[string]string{
		"ListViews": "the bounded ListViewPage read is the canonical session-list entrypoint",
	})
	forbidPackageFunctions(t, filepath.Join(root, "internal", "adapter", "workspacepath", "resolver.go"), map[string]string{
		"ResolveExistingDir": "the Resolver method is the canonical port implementation",
	})
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
// (including the reusable chathistory adapter contract). Domain has no Agent
// SDK exception: agentexec projects framework values into application-owned
// domain values at the boundary.
func TestDomainStaysFrameworkFree(t *testing.T) {
	root := moduleRoot(t)
	// componentPkg is banned here (not in frameworkImports) because application
	// legitimately imports internal/component/taskgroup — only the inner domain
	// ring must stay free of it, and layerOf leaves component unclassified so the
	// ring rule alone would miss a domain → component edge.
	forbidExternalImports(t, filepath.Join(root, "internal", "domain"),
		append([]string{componentPkg, "path/filepath"}, frameworkImports...))
}

// TestDomainDoesNotRenderAgentOrToolPresentation keeps model/tool text and
// filesystem-format helpers in adapters. Domain may return semantic values and
// structured verdicts, but it cannot regain the presentation functions moved
// during the boundary closure.
func TestDomainDoesNotRenderAgentOrToolPresentation(t *testing.T) {
	root := moduleRoot(t)
	checks := map[string]map[string]string{
		filepath.Join(root, "internal", "domain", "agentmemory"): {
			"Render":         "memory prompt rendering belongs to adapter/agentexec",
			"EstimateTokens": "model token approximation belongs to adapter/agentexec",
			"NormalizeFacts": "LLM Markdown extraction belongs to adapter/maintenance",
		},
		filepath.Join(root, "internal", "domain", "plan"): {
			"Render": "plan prompt/tool formatting belongs to adapter/planpresentation",
		},
		filepath.Join(root, "internal", "domain", "skills"): {
			"ProjectDir": "skill source layout belongs to adapter/promptsource",
			"Info":       "discovered-skill client projection belongs to application/workspace",
		},
		filepath.Join(root, "internal", "domain", "approval"): {
			"RiskFor": "approval-risk wording belongs to adapter/agentexec",
		},
		filepath.Join(root, "internal", "domain", "tool"): {
			"BypassImmuneReason": "tool refusal wording belongs to adapter/agentexec",
		},
	}
	for dir, banned := range checks {
		forbidTopLevelNames(t, dir, banned)
	}
}

// TestComponentStaysDomainFree keeps the internal/component ring free of DOMAIN
// coupling: components are no-domain-semantics primitives (signal / taskgroup /
// pathidentity / httporigin) that both application and
// adapter reuse. layerOf leaves component unclassified, so the ring rule catches
// only the INBOUND domain → component edge; this covers the OUTBOUND one.
func TestComponentStaysDomainFree(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "component"), []string{domainPkg})
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

// TestAppDoesNotInterpretAgentContinuationState preserves the consumer
// boundary: App stores the Agent snapshot as an opaque value, while only the
// Agent runtime may decode Suspension.FrameworkState.
func TestAppDoesNotInterpretAgentContinuationState(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || identifier.Name != "FrameworkState" {
				return true
			}
			relativePath, _ := filepath.Rel(root, path)
			t.Errorf("App production code interprets Agent Suspension.FrameworkState: %s", relativePath)
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk App runtime: %v", walkErr)
	}
}

// TestAgentSDKStaysBehindExecutionAdapters keeps the reusable framework at the
// App's anti-corruption edge. agentexec translates process/runtime values;
// toolset implements framework tool capabilities. Domain, application,
// delivery, infrastructure, and unrelated adapters must see only App-owned
// values and ports.
func TestAgentSDKStaysBehindExecutionAdapters(t *testing.T) {
	root := moduleRoot(t)
	allowed := []string{
		filepath.Join(root, "internal", "adapter", "agentexec"),
		filepath.Join(root, "internal", "adapter", "toolset"),
	}
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath != "github.com/Tangerg/lynx/agent" &&
				!strings.HasPrefix(importPath, "github.com/Tangerg/lynx/agent/") {
				continue
			}
			contained := false
			for _, dir := range allowed {
				if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
					contained = true
					break
				}
			}
			if !contained {
				relativePath, _ := filepath.Rel(root, path)
				t.Errorf("Agent SDK import escaped execution adapters: %s imports %q", relativePath, importPath)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk App runtime: %v", walkErr)
	}
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

// TestDeliveryDoesNotWireApplicationCollaborators keeps construction cycles and
// background execution ownership in Bootstrap. Delivery can invoke a schedule
// use case and project an accepted firing, but it must not bind a Runner, build
// a worker/launcher, or start the worker loop itself.
func TestDeliveryDoesNotWireApplicationCollaborators(t *testing.T) {
	root := moduleRoot(t)
	forbidSelectorCalls(t, filepath.Join(root, "internal", "delivery", "server"), map[string]string{
		"BindRunner":        "post-construction schedule wiring is forbidden",
		"NewRunLauncher":    "Bootstrap owns scheduled-run launcher construction",
		"NewWorker":         "Bootstrap owns background worker construction",
		"RunWorker":         "Bootstrap owns background worker lifetime",
		"StartScheduledRun": "the schedule application owns scheduled Run starts",
	})
	forbidQualifiedCalls(t, filepath.Join(root, "internal", "delivery", "server"), map[string]string{
		"schedules.New":        "Bootstrap owns schedule coordinator construction",
		"workspace.NewContext": "Bootstrap owns workspace use-case construction",
		"codebase.New":         "Bootstrap owns codebase coordinator construction",
	})
}

// TestAmbientRuntimePathsStayAtProcessComposition prevents every inner runtime
// ring from independently rediscovering the user home or process cwd. Those
// values are one process snapshot: Bootstrap and adapters consume explicit
// inputs so persistence, sandboxing, hooks, prompts, workspace reads, schedules,
// transport state, and server metadata cannot drift.
func TestAmbientRuntimePathsStayAtProcessComposition(t *testing.T) {
	root := moduleRoot(t)
	forbidQualifiedCalls(t, filepath.Join(root, "internal"), map[string]string{
		"os.UserHomeDir": "the process composition root owns the user-home snapshot",
		"os.Getwd":       "the process composition root owns the launch-directory snapshot",
		"filepath.Abs":   "inner runtime paths must resolve against an explicit absolute root",
	})
}

// TestDeliveryDoesNotBypassWorkspaceUseCases keeps filesystem path handling,
// file/Git reads, and prompt-source discovery behind application/workspace.
// Delivery may project their values to protocol, but must not reach a concrete
// adapter to complete a workspace request.
func TestDeliveryDoesNotBypassWorkspaceUseCases(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery"), []string{
		"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace",
		"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath",
		"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource",
		"github.com/fsnotify/fsnotify",
	})
}

// TestDeliveryServerDoesNotOwnFilesystemTechnology keeps filesystem traversal,
// path policy, and file-notification lifecycle in the workspace application
// and its adapter. Server handlers may project use-case values only.
func TestDeliveryServerDoesNotOwnFilesystemTechnology(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery", "server"), []string{
		"os",
		"path/filepath",
		"io/fs",
		"github.com/fsnotify/fsnotify",
	})
}

// TestProductSessionsDoNotCarryAgentContinuation prevents agent-core identity
// and opaque continuation JSON from drifting back into the Session bounded
// context. The checkpoint aggregate owns opaque executor state; the product
// domain owns conversation identity, user-created fork lineage and presentation.
func TestProductSessionsDoNotCarryAgentContinuation(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "domain", "session")
	forbiddenFields := map[string]struct{}{
		"UserID": {}, "AgentName": {}, "Metadata": {}, "DelegationMetadata": {},
	}
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			if strings.HasPrefix(strings.Trim(imp.Path.Value, `"`), "github.com/Tangerg/lynx/agent/") {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: product sessions must not import Agent runtime packages", rel)
			}
		}
		file, err = parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok {
				return true
			}
			for _, name := range field.Names {
				if _, forbidden := forbiddenFields[name.Name]; forbidden {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: product Session field %s leaks Agent continuation state", rel, name.Name)
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk session domain: %v", walkErr)
	}
}

// TestDeliveryDoesNotOwnModelPolicy keeps the static catalog behind the
// application/models coordinator. Delivery maps policy results to the protocol;
// it must not enumerate catalog data or duplicate provider capability rules.
func TestDeliveryDoesNotOwnModelPolicy(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery", "server"),
		[]string{"github.com/Tangerg/lynx/models/catalog"})
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
// decode ProcessSnapshot continuation payloads, or record framework aggregate
// usage directly. Managed interaction owns those execution mechanics; App
// observes its boundaries and owns the detailed accounting projection.
// TestAgentExecDelegatesManagedExecution pins what the rule is actually about:
// the framework's managed interaction drives the tool loop and records framework
// usage, so this adapter must never construct a runner or record usage itself.
// The prohibition is stated per behavior rather than as a ban on naming the
// package, because touching a pure value helper there — the manifest projection
// that pairs with PromoteTools — drives nothing and records nothing. Only
// NewRunner can produce a Runner, so forbidding it forbids driving a loop.
func TestAgentExecDelegatesManagedExecution(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "adapter", "agentexec")

	forbiddenSelectors := map[string]string{
		"toolloop.NewRunner": "managed interaction owns the ToolLoop runner",
		"pc.RecordUsage":     "managed interaction owns framework usage recording",
		"proc.RecordUsage":   "managed interaction owns framework usage recording",
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
// adapters through local configuration and the narrow tool.Tool capability;
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
// assembles and closes — it must not become a business facade or hide an adapter
// implementation behind an unexported receiver. Assembly/config/seed functions
// are fine; Host.Close is the only receiver method Bootstrap may own.
func TestBootstrapExposesNoBusinessMethod(t *testing.T) {
	root := moduleRoot(t)
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
			if !ok || fn.Recv == nil {
				continue // plain assembly/configuration functions are fine
			}
			if receiverName(fn.Recv) == "Host" && fn.Name.Name == "Close" {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: bootstrap receiver method %s.%s is forbidden — move behavior to application or an adapter (§16 rule 8)", rel, receiverName(fn.Recv), fn.Name.Name)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk bootstrap: %v", walkErr)
	}
}

// TestBootstrapDoesNotOwnLiveRuntimeState keeps the composition root limited to
// startup loading and assembly. Long-lived synchronization, fallback policy,
// and adapter projections belong to their owning application or adapter type.
func TestBootstrapDoesNotOwnLiveRuntimeState(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "bootstrap")
	forbidExternalImports(t, dir, []string{"sync/atomic"})
	forbidTopLevelNames(t, dir, map[string]string{
		"buildUtilityEnvironment":   "utility role resolution belongs to modelclient",
		"buildEmbeddingEnvironment": "embedding role resolution belongs to modelclient",
		"liveStateSnapshot":         "maintenance live-state projection belongs to the maintenance adapter",
	})
}

// TestApplicationCoordinatorsDoNotExposeAtomicState makes live-state
// synchronization an implementation detail of RoleState and ToolPolicyState,
// rather than a cross-boundary constructor dependency.
func TestApplicationCoordinatorsDoNotExposeAtomicState(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range []string{
		filepath.Join(root, "internal", "application", "models", "coordinator.go"),
		filepath.Join(root, "internal", "application", "integrations", "coordinator.go"),
	} {
		forbidExternalImports(t, path, []string{"sync/atomic"})
	}
}

func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	typ := recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	id, _ := typ.(*ast.Ident)
	if id == nil {
		return ""
	}
	return id.Name
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

// TestDomainValuesCarryNoJSONTags keeps serialization ownership at the adapter
// boundary. Domain values may define semantic JSON *values* (for example an
// input schema), but their Go structs must not carry a transport/storage field
// shape through json tags.
func TestDomainValuesCarryNoJSONTags(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "domain")
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
		ast.Inspect(f, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok || field.Tag == nil || !strings.Contains(field.Tag.Value, `json:`) {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: domain value carries wire/storage JSON tag %s", rel, field.Tag.Value)
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk domain: %v", walkErr)
	}
}

// TestDeliveryDoesNotOwnArchiveRecoveryOrValidation keeps the archive document
// decoder mechanical. Canonical snapshot validation, terminal-state recovery,
// and durable mutation recovery are application concerns; Delivery only maps
// the versioned wire document to/from the application portable model.
func TestDeliveryDoesNotOwnArchiveRecoveryOrValidation(t *testing.T) {
	root := moduleRoot(t)
	forbidSelectorCalls(t, filepath.Join(root, "internal", "delivery", "server"), map[string]string{
		"NormalizeForRestore":       "archive normalization belongs to application/sessions",
		"ValidateToolResults":       "archive structural validation belongs to application/sessions",
		"CanonicalSnapshot":         "terminal archive derivation belongs to application/sessions",
		"RecoverWorkspaceMutations": "startup recovery belongs to the composition root",
	})
}

// TestDeliveryDoesNotAuthorDomainText keeps presentation out of the business of
// explaining a run to a person. Why a run stopped is a domain fact, and the
// sentence a reader sees is the client's to write in the reader's language;
// presentation carries the detail the domain reported, including the absence of
// one.
//
// The run encoders once defaulted that sentence per kind, which is why the rule
// is mechanical rather than a naming ban: the prose hid one call away, in helpers
// that only looked like mappers. So in these files a literal may only be a
// programmer's diagnostic — anything that can reach a client has to come from a
// typed value, including the problem's own symbol.
//
// The MCP and provider projections are here for the same reason: they turned a
// domain connection state into an English sentence, which is the same leak on a
// surface where the client already renders by symbol.
func TestDeliveryDoesNotAuthorDomainText(t *testing.T) {
	root := moduleRoot(t)
	server := filepath.Join(root, "internal", "delivery", "server")
	for _, name := range []string{
		"presenter_run.go", "artifact_encode.go", "mcp_projection.go", "providers.go",
	} {
		forbidAuthoredText(t, filepath.Join(server, name),
			"a run's explanation is authored where the case is known and worded by the client")
	}
}

// TestGoalReasonStaysMachineReadable prevents Delivery from collapsing typed
// stopping context back into one localized sentence. The client needs the code
// for behavior and localization, while detail remains independently available.
func TestGoalReasonStaysMachineReadable(t *testing.T) {
	reason, ok := reflect.TypeFor[protocol.Goal]().FieldByName("Reason")
	if !ok {
		t.Fatal("protocol.Goal no longer exposes stopping context")
	}
	if want := reflect.TypeFor[*protocol.GoalReason](); reason.Type != want {
		t.Errorf("protocol.Goal.Reason = %s, want %s", reason.Type, want)
	}
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "delivery", "server"), map[string]string{
		"goalReason": "a plain-text reason loses the stable code and client localization boundary",
	})
}

// TestDeliveryServerDependsOnUseCaseBoundaries prevents concrete execution,
// persistence, or composition types from entering the protocol implementation.
// Adapter imports remain available to other delivery packages where a transport
// integration genuinely needs them; the server itself translates only between
// protocol values and application/domain ports.
func TestDeliveryServerDependsOnUseCaseBoundaries(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery", "server"), []string{
		"github.com/Tangerg/lynx/app/runtime/internal/adapter",
		"github.com/Tangerg/lynx/app/runtime/internal/infra",
		"github.com/Tangerg/lynx/app/runtime/internal/bootstrap",
		"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency",
	})
}

// TestDeliveryDoesNotImplementQuerySemantics keeps delivery out of deciding which
// rows a read returns, in what order, and where a page stops. Those are
// properties of the query: a correct cursor encodes the sort position and the
// normalized filters it was minted for, which the query knows and a presenter
// does not. The one that lived here took a materialized slice and searched it for
// an element id, so every paged read loaded its whole collection to return a
// slice of it.
//
// Delivery may still name a refused page request — it maps the sentinel onto
// invalid_params — but it may not run the mechanics.
func TestDeliveryDoesNotImplementQuerySemantics(t *testing.T) {
	root := moduleRoot(t)
	server := filepath.Join(root, "internal", "delivery", "server")
	forbidTopLevelNames(t, server, map[string]string{
		"pageByCursor":            "cutting a page belongs to the read that ordered the rows",
		"defaultItemPageLimit":    "how wide a page may be is the read's policy",
		"defaultSessionPageLimit": "how wide a page may be is the read's policy",
	})
	forbidQualifiedCalls(t, server, map[string]string{
		"keyset.Decode": "a cursor is decoded by the read that minted it",
		"keyset.Encode": "a cursor is minted by the read that knows the sort position",
		"keyset.Limit":  "how wide a page may be is the read's policy",
		"keyset.PageOf": "cutting a page belongs to the read that ordered the rows",
	})
}

// TestDeliveryDoesNotDeriveSessionActivity keeps precedence between active
// admission and durable interrupt state in the sessions read model. Delivery
// maps the resulting enum but cannot duplicate the precedence rule.
func TestDeliveryDoesNotDeriveSessionActivity(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "delivery", "server"), map[string]string{
		"liveStatus":        "session activity is an application read model",
		"runningSessionSet": "active-run lookup is an application read model",
		"waitingSessionSet": "interrupt lookup is an application read model",
	})
}

// TestDeliveryServerExportsOnlyItsContract keeps internal event ingress and
// orchestration seams off the concrete Server API. Composition may close the
// Server; every other exported method must belong to protocol.Runtime.
func TestDeliveryServerExportsOnlyItsContract(t *testing.T) {
	serverType := reflect.TypeFor[*deliveryserver.Server]()
	runtimeType := reflect.TypeFor[protocol.Runtime]()
	if !serverType.Implements(runtimeType) {
		t.Fatal("delivery Server no longer implements protocol.Runtime")
	}
	allowed := map[string]struct{}{"Close": {}}
	for method := range runtimeType.Methods() {
		allowed[method.Name] = struct{}{}
	}
	for method := range serverType.Methods() {
		if _, ok := allowed[method.Name]; !ok {
			t.Errorf("delivery Server exports non-contract method %s", method.Name)
		}
	}
}

// TestWorkspaceChangeNoticeBelongsToWorkspace prevents the Run use case from
// becoming the vocabulary owner for a workspace subscription signal.
func TestWorkspaceChangeNoticeBelongsToWorkspace(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "application", "runs"), map[string]string{
		"FileChange":       "workspace change scope belongs to application/workspace",
		"FileChangeNotice": "workspace change scope belongs to application/workspace",
	})
}

// TestStartCommandHasOneInputRepresentation prevents parallel opening-message,
// media, and text fields from returning beside ContentBlock. Application owns
// the one materialization into the executor-facing turn request.
func TestStartCommandHasOneInputRepresentation(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "application", "runs", "commands.go")
	for _, field := range []string{"Message", "Media", "OpeningUserText"} {
		if got := namedStructFieldTypeOptional(t, path, "StartCommand", field); got != "" {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: StartCommand.%s = %s duplicates ContentBlock input", rel, field, got)
		}
	}
}

// TestTranscriptItemUsesOneNeutralDomainTimestamp keeps wire vocabulary out of
// the durable union. Delivery maps OccurredAt to createdAt for message-like
// variants and startedAt for ToolCall; the domain must not store both names or
// call a ToolCall start a creation time.
func TestTranscriptItemUsesOneNeutralDomainTimestamp(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "domain", "execution", "transcript", "model.go")
	if got := namedStructFieldTypeOptional(t, path, "Item", "OccurredAt"); got != "time.Time" {
		t.Errorf("transcript.Item.OccurredAt = %q, want time.Time", got)
	}
	if got := namedStructFieldTypeOptional(t, path, "Item", "CreatedAt"); got != "" {
		t.Errorf("transcript.Item.CreatedAt = %q; variant-specific time belongs to Delivery", got)
	}
}

// TestDeliveryPortsDoNotKeepFormerTestOrchestrationMethods prevents a test
// setup convenience from widening the production consumer ports. Admission
// probes, raw restore, and registry lookups belong to their owning application
// coordinators; handlers consume only the use cases they actually drive.
func TestDeliveryPortsDoNotKeepFormerTestOrchestrationMethods(t *testing.T) {
	root := moduleRoot(t)
	const reason = "not a production handler dependency"
	forbidInterfaceMethods(t, filepath.Join(root, "internal", "delivery", "server", "application_ports.go"),
		map[string]map[string]string{
			"sessionUseCases": {
				"ClaimWorkingTreeMutation": reason, "ClaimWorkingTreeRun": reason, "RestoreSession": reason,
			},
			"integrationUseCases": {"MCPRegisteredServer": reason, "MCPServerStatus": reason},
			"modelUseCases":       {"DefaultModel": reason},
			"runUseCases": {
				"AcquireSession": reason, "ActiveSession": reason,
				"ActiveSessions": reason, "Contains": reason,
			},
		})
}

// TestDeliveryReadsRunsFromDurableProjection keeps the live registry out of the
// answer to "which Runs exist and what are they doing". The registry tracks the
// segments THIS process is streaming, so it reports a different set after a
// restart and never holds a Run parked on an interrupt — it answers a narrower
// question than the one runs.list asks. Delivery reads the durable admission
// record through the query port instead.
func TestDeliveryReadsRunsFromDurableProjection(t *testing.T) {
	root := moduleRoot(t)
	forbidInterfaceMethods(t, filepath.Join(root, "internal", "delivery", "server", "application_ports.go"),
		map[string]map[string]string{
			"runUseCases": {
				"List": "the set of Runs is a durable projection, not this process's live registry",
			},
		})
}

// forbidInterfaceMethods rejects named methods on named interfaces in one file.
// It guards consumer-port width: which use cases a ring is allowed to drive is a
// dependency decision, and an extra method is how one ring quietly starts owning
// another's read model.
func forbidInterfaceMethods(t *testing.T, path string, banned map[string]map[string]string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			named, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			blocked, watched := banned[named.Name.Name]
			if !watched {
				continue
			}
			iface, ok := named.Type.(*ast.InterfaceType)
			if !ok || iface.Methods == nil {
				t.Fatalf("%s: %s is not an interface", path, named.Name.Name)
			}
			for _, method := range iface.Methods.List {
				for _, name := range method.Names {
					if reason, leaked := blocked[name.Name]; leaked {
						t.Errorf("%s: %s.%s is %s", path, named.Name.Name, name.Name, reason)
					}
				}
			}
		}
	}
}

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

// TestRuntimeInterruptValuesStayWireFree keeps the application interrupt plan
// and the domain resume decision free of the JSON shape used by persisted agent
// suspensions. The agent adapter owns that codec; these values retain only
// business vocabulary.
func TestRuntimeInterruptValuesStayWireFree(t *testing.T) {
	root := moduleRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "application", "runs", "interrupt_contract.go"),
		filepath.Join(root, "internal", "domain", "execution", "interrupts", "resolution.go"),
	}
	for _, path := range paths {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == "encoding/json" {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: runtime interrupt value must not import encoding/json", rel)
			}
		}
		ast.Inspect(f, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok || field.Tag == nil || !strings.Contains(field.Tag.Value, `json:`) {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: runtime interrupt value must not carry JSON tag %s", rel, field.Tag.Value)
			return true
		})
	}
}

// TestRememberScopeUsesApprovalDomainType prevents a second raw-string scope
// vocabulary from growing beside approval.Scope at the application/agent seam.
func TestRememberScopeUsesApprovalDomainType(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		path       string
		structName string
	}{
		{filepath.Join(root, "internal", "application", "runs", "commands.go"), "ApprovalResponse"},
		{filepath.Join(root, "internal", "domain", "execution", "interrupts", "resolution.go"), "Resolution"},
	}
	for _, check := range checks {
		if got := namedStructFieldType(t, check.path, check.structName, "RememberScope"); got != "approval.Scope" {
			rel, _ := filepath.Rel(root, check.path)
			t.Errorf("%s: %s.RememberScope = %s, want approval.Scope", rel, check.structName, got)
		}
	}
}

// TestRunLifecycleStateStaysConcrete prevents the registry and journal from
// regaining one-use type parameters. A second production payload would be
// evidence for a deliberately redesigned abstraction, not a silent generality
// increase to these lifecycle owners.
func TestRunLifecycleStateStaysConcrete(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		path string
		name string
	}{
		{filepath.Join(root, "internal", "application", "runs", "registry.go"), "registry"},
		{filepath.Join(root, "internal", "application", "runs", "journal.go"), "Journal"},
	}
	for _, check := range checks {
		f, err := parser.ParseFile(token.NewFileSet(), check.path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", check.path, err)
		}
		found := false
		for _, decl := range f.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				named, ok := spec.(*ast.TypeSpec)
				if !ok || named.Name.Name != check.name {
					continue
				}
				found = true
				if named.TypeParams != nil && len(named.TypeParams.List) > 0 {
					rel, _ := filepath.Rel(root, check.path)
					t.Errorf("%s: %s must stay concrete over its only production payload", rel, check.name)
				}
			}
		}
		if !found {
			t.Fatalf("%s: type %s not found", check.path, check.name)
		}
	}
}

// TestTurnControlHasNoProducerDispatcherPort keeps the removed fat dispatcher
// interface from returning. The concrete turn implementation has consumers,
// and each consumer defines only the slice it needs.
func TestTurnControlHasNoProducerDispatcherPort(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "adapter", "agentexec", "turn")
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
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				named, ok := spec.(*ast.TypeSpec)
				if !ok || named.Name.Name != "Dispatcher" {
					continue
				}
				if _, interfaceType := named.Type.(*ast.InterfaceType); interfaceType {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: producer-owned turn.Dispatcher interface must not return", rel)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk turn adapter: %v", walkErr)
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

// componentPkg is the neutral concurrency/wiring primitive package
// (signal / taskgroup). layerOf leaves it unclassified so the
// ring rule doesn't check edges into it; the domain rings ban it explicitly
// (application/delivery/composition may import it). Prefix-matched.
const componentPkg = "github.com/Tangerg/lynx/app/runtime/internal/component"

// domainPkg is the bounded-context ring. internal/component must not import it
// (components are no-domain-semantics primitives); layerOf leaves component
// unclassified, so the ring rule alone would miss the outbound edge. Prefix-matched.
const domainPkg = "github.com/Tangerg/lynx/app/runtime/internal/domain"

// protocolPkg is the wire-type package; it must stay pure wire (no domain /
// application import) so protocol types never leak inward (§16 rule 10).
const protocolPkg = "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// externalSDKs are the external agent-SDK / driver / framework libraries the
// inner + delivery rings must never import directly (the internal infra edges are
// covered by the ring rule). Prefix-matched.
var externalSDKs = []string{
	"github.com/Tangerg/lynx/agent",
	"github.com/fsnotify/fsnotify",
	"modernc.org/sqlite",
	"github.com/go-git",
	"github.com/mark3labs",
	"github.com/sourcegraph",
}

// frameworkImports are the framework / driver / SDK / format-codec packages an
// inner ring must never import. Prefix-matched, so e.g. "modernc.org/sqlite"
// catches the driver and its sub-packages. Content codecs belong to adapters or
// infrastructure, not to Domain or Application values.
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
	"github.com/Tangerg/lynx/models/catalog",
	"gopkg.in/yaml.v3",
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

// forbidSelectorCalls rejects direct calls whose selector name belongs to a
// forbidden construction or lifecycle operation. The package receiver does not
// matter here: these names are intentionally specific to the ownership seams
// guarded above.
func forbidSelectorCalls(t *testing.T, dir string, banned map[string]string) {
	t.Helper()
	root := moduleRoot(t)
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
		ast.Inspect(f, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			reason, forbidden := banned[selector.Sel.Name]
			if !forbidden {
				return true
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: delivery calls %s; %s", rel, selector.Sel.Name, reason)
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
}

// forbidTopLevelNames rejects production functions and named types with names
// that would reintroduce a removed ownership seam.
func forbidTopLevelNames(t *testing.T, dir string, banned map[string]string) {
	t.Helper()
	root := moduleRoot(t)
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
				if reason, forbidden := banned[decl.Name.Name]; forbidden {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: removed ownership seam %s returned; %s", rel, decl.Name.Name, reason)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					var names []*ast.Ident
					switch named := spec.(type) {
					case *ast.TypeSpec:
						names = []*ast.Ident{named.Name}
					case *ast.ValueSpec:
						names = named.Names
					}
					for _, name := range names {
						if reason, forbidden := banned[name.Name]; forbidden {
							rel, _ := filepath.Rel(root, path)
							t.Errorf("%s: removed ownership seam %s returned; %s", rel, name.Name, reason)
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

// forbidPackageFunctions rejects package functions while allowing a method
// with the same name. It is used when the method is the canonical port surface
// and a convenience function would duplicate that capability.
func forbidPackageFunctions(t *testing.T, path string, banned map[string]string) {
	t.Helper()
	root := moduleRoot(t)
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range f.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		if reason, forbidden := banned[fn.Name.Name]; forbidden {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: redundant package function %s returned; %s", rel, fn.Name.Name, reason)
		}
	}
}

// forbidAuthoredText rejects any string literal in a mapping file that is not a
// programmer's diagnostic. A presenter's job is to carry typed values across, so
// a literal that can reach a client is prose the presenter wrote itself —
// invisible to the locale catalogs and to whoever owns the real answer. Import
// paths, panic arguments, error messages and the empty string are exempt: none
// of them is copy.
func forbidAuthoredText(t *testing.T, path, reason string) {
	t.Helper()
	root := moduleRoot(t)
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	diagnostic := make(map[ast.Expr]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		switch exprString(call.Fun) {
		case "panic", "errors.New", "fmt.Errorf":
			diagnostic[call.Args[0]] = struct{}{}
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		if _, isImport := node.(*ast.ImportSpec); isImport {
			return false
		}
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING || literal.Value == `""` {
			return true
		}
		if _, exempt := diagnostic[literal]; exempt {
			return true
		}
		rel, _ := filepath.Rel(root, path)
		t.Errorf("%s: authored text %s; %s", rel, literal.Value, reason)
		return true
	})
}

// forbidQualifiedCalls rejects a named package-selector call while allowing
// unrelated methods with the same selector. It guards composition ownership
// seams such as delivery's application-coordinator constructors.
func forbidQualifiedCalls(t *testing.T, dir string, banned map[string]string) {
	t.Helper()
	root := moduleRoot(t)
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
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := exprString(selector.X) + "." + selector.Sel.Name
			if reason, forbidden := banned[name]; forbidden {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: delivery calls %s; %s", rel, name, reason)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
}

// namedStructFieldType returns one named struct field's rendered type. It keeps
// value-object vocabulary assertions AST-based instead of depending on source
// formatting or comments.
func namedStructFieldType(t *testing.T, path, structName, fieldName string) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			named, ok := spec.(*ast.TypeSpec)
			if !ok || named.Name.Name != structName {
				continue
			}
			value, ok := named.Type.(*ast.StructType)
			if !ok || value.Fields == nil {
				t.Fatalf("%s: %s is not a struct", path, structName)
			}
			for _, field := range value.Fields.List {
				for _, name := range field.Names {
					if name.Name == fieldName {
						return exprString(field.Type)
					}
				}
			}
			t.Fatalf("%s: %s.%s not found", path, structName, fieldName)
		}
	}
	t.Fatalf("%s: type %s not found", path, structName)
	return ""
}

// namedStructFieldTypeOptional is namedStructFieldType for an intentionally
// absent field; it returns an empty string when the named struct or field is not
// present, while still failing on an unreadable source file.
func namedStructFieldTypeOptional(t *testing.T, path, structName, fieldName string) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			named, ok := spec.(*ast.TypeSpec)
			if !ok || named.Name.Name != structName {
				continue
			}
			value, ok := named.Type.(*ast.StructType)
			if !ok || value.Fields == nil {
				t.Fatalf("%s: %s is not a struct", path, structName)
			}
			for _, field := range value.Fields.List {
				for _, name := range field.Names {
					if name.Name == fieldName {
						return exprString(field.Type)
					}
				}
			}
			return ""
		}
	}
	t.Fatalf("%s: type %s not found", path, structName)
	return ""
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

// TestSystemInvariantsStayInApplication keeps cross-resource invariants out of
// the wire ring (vNext plan D3). A SystemInvariantSpec names a fact that spans
// runs, interrupts and the store, and it points at the transaction responsible
// for it — neither of which delivery can see. Two things go wrong if delivery
// names one: the wire layer starts asserting business facts, and the application
// ring would have to import delivery to register, which is an outward edge.
func TestSystemInvariantsStayInApplication(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "delivery"), map[string]string{
		"SystemInvariantSpec": "a cross-resource invariant is named by the ring that owns its transaction",
		"TransactionBoundary": "a transaction boundary is an application concern",
	})
	// The declared set must be actionable wherever it lives.
	if err := appcontract.Validate(); err != nil {
		t.Fatalf("declared system invariants: %v", err)
	}
	if len(appcontract.SystemInvariants()) == 0 {
		t.Fatal("no system invariant is declared; the manifest gate would pass vacuously")
	}
}
