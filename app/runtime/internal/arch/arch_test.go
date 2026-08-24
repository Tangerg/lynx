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
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	deliveryserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestPlanMutationHasOneOwner freezes the P16 Plan vertical: aggregate fields
// are closed, Tool adapters cannot regain a Store-shaped inbound boundary, and
// SQLite cannot assign time or invoke the aggregate transition itself.
func TestPlanMutationHasOneOwner(t *testing.T) {
	stateType := reflect.TypeFor[plan.State]()
	for index := range stateType.NumField() {
		field := stateType.Field(index)
		if field.IsExported() {
			t.Errorf("plan.State field %s is exported; replacements must enter through domain behavior", field.Name)
		}
	}
	root := moduleRoot(t)
	forbidTopLevelNames(t,
		filepath.Join(root, "internal", "adapter", "toolset", "builtin"),
		map[string]string{"Store": "Plan tools consume application use cases, not persistence"},
	)
	forbidSelectorCalls(t,
		filepath.Join(root, "internal", "infra", "sqlite", "plan.go"),
		map[string]string{
			"Now":     "the Plan use case supplies replacement time",
			"Replace": "the Plan aggregate decides replacements before persistence",
		},
	)
}

// TestSessionMutationHasOneOwner freezes the P16 Session vertical: aggregate
// fields stay closed and SQLite persists exact values without constructing,
// editing, forking, naming, or restoring product state itself.
func TestSessionMutationHasOneOwner(t *testing.T) {
	sessionType := reflect.TypeFor[session.Session]()
	for index := range sessionType.NumField() {
		field := sessionType.Field(index)
		if field.IsExported() {
			t.Errorf("session.Session field %s is exported; changes must enter through domain behavior", field.Name)
		}
	}

	root := moduleRoot(t)
	mutationFile := filepath.Join(root, "internal", "infra", "sqlite", "session_mutation.go")
	forbidExternalImports(t, mutationFile, []string{"time", "github.com/google/uuid"})
	forbidSelectorCalls(t, mutationFile, map[string]string{
		"Apply":                    "Session edits belong to the aggregate before persistence",
		"Fork":                     "Session forks belong to the aggregate before persistence",
		"NameIfUntitled":           "generated-title arbitration belongs to the aggregate and use case",
		"InstallRestoredWorkspace": "workspace admission belongs outside persistence",
		"ReplaceWithRestore":       "Session restore replacement belongs to the aggregate and use case",
	})
	forbidTopLevelNames(t, mutationFile, map[string]string{
		"Create":           "Session construction belongs to Domain/Application",
		"Ensure":           "opening idempotency must not derive Session state in persistence",
		"Restore":          "restore persists an application-decided exact replacement",
		"Patch":            "Session edits belong to Domain/Application",
		"Fork":             "Session lineage belongs to Domain/Application",
		"SetModel":         "field setters create a second mutation owner",
		"Rename":           "field setters create a second mutation owner",
		"RenameIfUntitled": "title arbitration belongs to Domain/Application",
		"SetCWD":           "workspace edits belong to Domain/Application",
		"SetFavorite":      "field setters create a second mutation owner",
	})
}

// TestDependencyRule enforces Clean Architecture's Dependency Rule for Runtime:
// source dependencies point INWARD, toward Domain and Application. Outer rings may
// depend on inner rings; the reverse — or a driven/adapter ring reaching up into
// the composition root — is forbidden. See doc/ARCHITECTURE.md §6.
//
// Rings (outer → inner):
//
//	composition    internal/bootstrap/**,        the "main" component: config load, assembly, host
//	               internal/config, cmd/**       lifecycle. Wires every ring, so it imports anything —
//	                                             but nothing imports IT.
//	protocol       protocol/**                   public binding-neutral values and strict validation
//	delivery       internal/delivery/**          operation, HTTP+SSE dispatch and transport
//	adapter        internal/adapter/**           capability adapters, incl. adapter/agentexec (the
//	                                              agent-execution adapter over the agent SDK)
//	application    internal/application/**        use-case coordinators (runs / sessions / capabilities /
//	                                              workspace / schedules) — engine- and wire-neutral
//	infra          internal/infra/**             sqlite / git / lsp / mcp / exec — driven adapters & frameworks
//	domain         internal/domain/**            bounded contexts: entities, values, invariants, pure policies
//
// Forbidden edges (an inner ring learning about an outer one, a driven ring
// reaching sideways/up, or anything importing the composition root):
//
//	domain      ↛ application, adapter, infra, delivery, composition
//	application ↛ adapter, infra, delivery, composition   (Application owns use cases, not implementations)
//	infra       ↛ application, adapter, delivery, composition   (technical mechanism: imports only domain)
//	adapter     ↛ delivery, composition
//	delivery    ↛ adapter, infra, composition   (drives Application, never implementations)
//
// Intentionally allowed inward / hexagonal edges (ARCHITECTURE.md §6):
//
//	application → domain          coordinators depend on entities + consumer-side ports
//	adapter → domain, application capability + agent-execution adapters implement application ports
//	adapter → adapter, infra      sibling adapters compose; capability adapters wrap driven capabilities
//	infra   → domain              technical mechanisms may use stable domain values
//	delivery → domain, application
//	composition → anything        the root wires every ring
//
// The ring rule is the backbone. Dedicated tests below cover the unclassified
// component umbrella, framework imports, wire isolation, and semantic ownership.
// The Go compiler enforces the remaining package DAG by rejecting import cycles.
func TestDependencyRule(t *testing.T) {
	root := moduleRoot(t)

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

		fileViolations, err := dependencyViolationsInFile(path, from)
		if err != nil {
			return err
		}
		for _, violation := range fileViolations {
			violations++
			t.Errorf("dependency-rule violation: %s (%s) imports %s (%s)", rel, from, violation.importPath, violation.toRing)
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

// TestOpaqueExecutorCheckpointConsumersDoNotModelFrameworkTrees keeps every
// checkpoint consumer outside agentexec byte-oriented. Runsegment and Bootstrap
// may bind, save, replace, or delete the opaque envelope, but tree shape and
// snapshot parsing remain exclusively in the Agent Framework ACL.
func TestOpaqueExecutorCheckpointConsumersDoNotModelFrameworkTrees(t *testing.T) {
	root := moduleRoot(t)
	for _, dir := range []string{
		filepath.Join(root, "internal", "adapter", "runsegment"),
		filepath.Join(root, "internal", "bootstrap"),
	} {
		walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, forbidden := range []string{
				"TreeSnapshot", "ProcessSnapshot", "ProcessSnapshots(", "ParseTreeSnapshot(",
				`json:"root_id"`, `json:"snapshots"`,
			} {
				if strings.Contains(string(source), forbidden) {
					relative, _ := filepath.Rel(root, path)
					t.Errorf("%s models opaque Agent Framework checkpoint state with %q", relative, forbidden)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("scan opaque executor checkpoint consumers: %v", walkErr)
		}
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
// (including the reusable chathistory adapter contract). Domain has no Agent
// SDK exception: agentexec projects framework values into application-owned
// domain values at the boundary.
func TestDomainStaysFrameworkFree(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "domain"),
		append(append([]string{"path/filepath"}, crossRingCapabilityImports...), frameworkImports...))
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
			"NormalizeFacts": "LLM Markdown extraction belongs to adapter/runmaintenance",
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

// TestSharedCapabilitiesStayPure keeps the few genuinely cross-ring mechanisms
// free of product and ring ownership. Each package names one proven shared
// capability; none may become a replacement common/component umbrella.
func TestSharedCapabilitiesStayPure(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{
		"completion", "httporigin", "idempotency",
	} {
		forbidExternalImports(t, filepath.Join(root, "internal", name), []string{
			domainPkg,
			"github.com/Tangerg/lynx/app/runtime/internal/application",
			"github.com/Tangerg/lynx/app/runtime/internal/adapter",
			"github.com/Tangerg/lynx/app/runtime/internal/infra",
			"github.com/Tangerg/lynx/app/runtime/internal/delivery",
			"github.com/Tangerg/lynx/app/runtime/internal/bootstrap",
		})
	}
}

// TestApplicationMechanismsStayApplicationOwned keeps mechanisms shared by
// multiple use cases inside the Application ring without letting them absorb
// Domain language or outward implementation concerns.
func TestApplicationMechanismsStayApplicationOwned(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{"opaquetoken", "pagination", "taskgroup"} {
		forbidExternalImports(t, filepath.Join(root, "internal", "application", name), []string{
			domainPkg,
			"github.com/Tangerg/lynx/app/runtime/internal/adapter",
			"github.com/Tangerg/lynx/app/runtime/internal/infra",
			"github.com/Tangerg/lynx/app/runtime/internal/delivery",
			"github.com/Tangerg/lynx/app/runtime/internal/bootstrap",
		})
	}
}

// TestApplicationStaysFrameworkFree enforces ARCHITECTURE.md §6.2's application-purity
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

// TestApplicationDoesNotInterpretExecutorContinuationState preserves the
// consumer boundary: Application stores executor snapshots as opaque values;
// only the execution adapter may decode framework state.
func TestApplicationDoesNotInterpretExecutorContinuationState(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(filepath.Join(root, "internal", "application"), func(path string, entry fs.DirEntry, err error) error {
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
			if !ok || (identifier.Name != "FrameworkState" && identifier.Name != "TreeSnapshot") {
				return true
			}
			relativePath, _ := filepath.Rel(root, path)
			t.Errorf("application production code interprets executor continuation state: %s", relativePath)
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk application runtime: %v", walkErr)
	}
}

// TestAgentFrameworkStaysBehindAgentexec keeps the Agent Framework at Runtime's
// single anti-corruption edge. Both the importing Runtime leaf and the imported
// Agent Framework packages are explicit: widening either set requires a reviewed contract
// decision instead of silently turning an umbrella prefix into an allowlist.
func TestAgentFrameworkStaysBehindAgentexec(t *testing.T) {
	const agentexecDir = "internal/adapter/agentexec"
	allowedImports := map[string]struct{}{
		"github.com/Tangerg/lynx/agent":             {},
		"github.com/Tangerg/lynx/agent/interaction": {},
	}
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
		if !strings.HasSuffix(path, ".go") {
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
			relativePath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relativePath = filepath.ToSlash(relativePath)
			relativeDir := filepath.ToSlash(filepath.Dir(relativePath))
			if relativeDir != agentexecDir && !strings.HasPrefix(relativeDir, agentexecDir+"/") {
				t.Errorf("Agent Framework import escaped %s: %s imports %q", agentexecDir, relativePath, importPath)
			}
			if _, allowed := allowedImports[importPath]; !allowed {
				t.Errorf("unreviewed Agent Framework package import: %s imports %q", relativePath, importPath)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk application runtime: %v", walkErr)
	}
}

// TestDomainStaysPure protects every bounded context in the innermost ring:
// it must not touch the filesystem, a SQL driver, HTTP, OTel, the agent SDK, or a
// shared runtime coordination primitives. (The accounting sub-context
// maps the SDK's token counts at the agentexec boundary, so it holds only the
// neutral core chat model, never agent/*.) The shared-capability ban is listed
// explicitly because layerOf intentionally leaves those exact packages unclassified.
func TestDomainStaysPure(t *testing.T) {
	root := moduleRoot(t)
	domain := filepath.Join(root, "internal", "domain")
	forbidExternalImports(t, domain, append(
		[]string{"os", "database/sql", "net", "net/http", "go.opentelemetry.io", "github.com/Tangerg/lynx/agent"},
		crossRingCapabilityImports...,
	))
	forbidTestImports(t, domain, []string{
		"github.com/Tangerg/lynx/app/runtime/internal/application",
		"github.com/Tangerg/lynx/app/runtime/internal/adapter",
		"github.com/Tangerg/lynx/app/runtime/internal/infra",
		"github.com/Tangerg/lynx/app/runtime/internal/delivery",
		"github.com/Tangerg/lynx/app/runtime/internal/bootstrap",
	})
}

// TestDeliveryStaysFrameworkFree keeps Delivery free of external implementation
// packages: no agent SDK, SQLite driver, Git, MCP, or LSP library directly (the
// ring rule already forbids the internal infra edge; this covers the EXTERNAL
// libraries). net/http is NOT banned — it is delivery's own transport.
func TestDeliveryStaysFrameworkFree(t *testing.T) {
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "delivery"), externalSDKs)
}

// TestDeliveryDoesNotControlAgentExecutions keeps complete Run commands behind
// the application/runs use-case surface. Delivery may decode and present wire
// data, but it must not plan, rebuild, assert, or steer concrete Agent execution
// handles.
func TestDeliveryDoesNotControlAgentExecutions(t *testing.T) {
	const agentExecPkg = "github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	root := moduleRoot(t)
	delivery := filepath.Join(root, "internal", "delivery")
	forbidExternalImports(t, delivery, []string{agentExecPkg})
	forbidTestImports(t, delivery, []string{agentExecPkg})
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
		"schedules.New":      "Bootstrap owns schedule coordinator construction",
		"workspace.NewScope": "Bootstrap owns workspace use-case construction",
		"codebase.New":       "Bootstrap owns codebase coordinator construction",
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

// TestApplicationDoesNotDependOnConcreteAgentEngine keeps the Agent runtime
// behind Bootstrap and the execution adapter. Application owns consumer-side
// ports and must not regain a dependency on the concrete agentexec Engine or
// one of its implementation subpackages.
func TestApplicationDoesNotDependOnConcreteAgentEngine(t *testing.T) {
	const agentExecPkg = "github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	root := moduleRoot(t)
	forbidExternalImports(t, filepath.Join(root, "internal", "application"), []string{agentExecPkg})
}

// TestAgentExecDelegatesManagedExecution locks the Framework/Host ownership
// boundary. The agent adapter may supply product prompts, pricing, observers,
// tools, and responses, but it must not rebuild the framework's ToolLoop,
// decode ProcessSnapshot continuation payloads, or record framework aggregate
// usage directly. Managed interaction owns those execution mechanics; Application
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
// may own only construction and process-lifecycle behavior. The package-private
// application capsule is a closed set of delivery/startup/worker composition
// operations; domain commands still belong to Application.
func TestBootstrapExposesNoBusinessMethod(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	allowed := map[string]map[string]bool{
		"Host": {"Close": true},
		"hostApplication": {
			"recoverStartup": true, "newOperationService": true,
			"openOperationDelivery": true, "notifyExternalChange": true,
			"startWorkers": true,
		},
		"hostWorkers": {"runOwnershipRecovery": true},
		"operationDelivery": {
			"beginShutdown": true, "awaitShutdown": true,
		},
		"Instance":       {"Close": true, "Endpoint": true, "ServerInfo": true},
		"InstanceConfig": {"validate": true},
	}
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
			receiver := receiverName(fn.Recv)
			if allowed[receiver][fn.Name.Name] {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: bootstrap receiver method %s.%s is forbidden — move business behavior to application or an adapter (§16 rule 8)", rel, receiver, fn.Name.Name)
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
		"liveStateSnapshot":         "Run maintenance live-state projection belongs to adapter/runmaintenance",
	})
}

// TestApplicationCoordinatorsDoNotExposeAtomicState makes live-state
// synchronization an implementation detail of RoleState and ToolPolicyState,
// rather than a cross-boundary constructor dependency.
func TestApplicationCoordinatorsDoNotExposeAtomicState(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range []string{
		filepath.Join(root, "internal", "application", "models", "coordinator.go"),
		filepath.Join(root, "internal", "application", "mcp", "coordinator.go"),
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
	structs, walkErr := collectStructDeclarations(dir)
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
		"goalDriver",
		"mcpCoordinator",
		"codebaseCoordinator",
		"runCoordinator",
		"executor",
		"runEffectTasks",
		"toolResources",
		"hostResources",
	} {
		if _, ok := lifetime[required]; !ok {
			t.Errorf("bootstrap.hostLifetime must own %s", required)
		}
	}
	if _, ok := lifetime["engine"]; ok {
		t.Error("bootstrap.hostLifetime owns engine; Agent execution must not be a resource closer")
	}
}

func collectStructDeclarations(root string) (map[string]*ast.StructType, error) {
	structs := make(map[string]*ast.StructType)
	err := walkProductionGoFiles(root, func(_ string, file *ast.File) error {
		for _, declaration := range file.Decls {
			general, isTypeDeclaration := declaration.(*ast.GenDecl)
			if !isTypeDeclaration || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				named, isNamedType := spec.(*ast.TypeSpec)
				if !isNamedType {
					continue
				}
				if structure, isStruct := named.Type.(*ast.StructType); isStruct {
					structs[named.Name.Name] = structure
				}
			}
		}
		return nil
	})
	return structs, err
}

// TestDeliveryHoldsNoRunLifecycleState enforces §16 rule 5: the delivery Server
// (the protocol handler) drives the run coordinator as a use-case surface, but
// must not itself HOLD the run registry, a cancel func, a task group, or a
// checkpoint store — the run-lifecycle ownership §20 moved to the application/Host.
// Two forms: (a) the Application task group is import-forbidden outright (a
// field would need the import; this also catches a held cancel-func group); (b)
// a struct-field AST walk forbids a held checkpoint store or run registry,
// whose packages the Server imports for other reasons (adapter/workspace's
// GitAvailable probe; application/runs' Coordinator + Event).
func TestDeliveryHoldsNoRunLifecycleState(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "delivery", "server")
	forbidExternalImports(t, dir, []string{"github.com/Tangerg/lynx/app/runtime/internal/application/taskgroup"})

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
	forbidExternalImports(t, filepath.Join(root, "protocol"),
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

// TestDomainDoesNotOwnConcreteToolInventory keeps model-facing names and
// argument schemas with toolset. Domain approval consumes only a safety class,
// tool identity, and a subject already derived by the concrete tool owner.
func TestDomainDoesNotOwnConcreteToolInventory(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal", "domain", "tool"), map[string]string{
		"SafetyClassFor":      "the concrete tool catalog owns name-to-safety classification",
		"ClassifiedToolNames": "the concrete tool catalog owns its completeness guard",
		"NameReadToolResult":  "a model-facing tool name belongs to its adapter",
	})

	forbidden := map[string]struct{}{
		"shell": {}, "read": {}, "write": {}, "edit": {}, "apply_patch": {},
		"web_fetch": {}, "read_tool_result": {}, "create_goal": {}, "propose_skill": {},
	}
	dir := filepath.Join(root, "internal", "domain", "approval")
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
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
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			if _, found := forbidden[value]; found {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s: approval domain names concrete tool %q; derive its semantics in toolset", relative, value)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk approval domain: %v", walkErr)
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

// TestDeliveryProjectionUsesOneVerb keeps outbound mapping under the existing
// presentX vocabulary. Inbound mappers may retain the explicit xFromWire form;
// xWire and xToWire make direction ambiguous and previously concealed duplicate
// projections behind different names.
func TestDeliveryProjectionUsesOneVerb(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "delivery", "server")
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
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
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			name := function.Name.Name
			if strings.HasSuffix(name, "FromWire") {
				continue
			}
			if strings.HasSuffix(name, "ToWire") || strings.HasSuffix(name, "Wire") {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s: projection helper %s must use presentX", relative, name)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk delivery server: %v", walkErr)
	}
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
		"github.com/Tangerg/lynx/app/runtime/internal/idempotency",
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
		"pagination.Decode": "a cursor is decoded by the read that minted it",
		"pagination.Encode": "a cursor is minted by the read that knows the sort position",
		"pagination.Limit":  "how wide a page may be is the read's policy",
		"pagination.PageOf": "cutting a page belongs to the read that ordered the rows",
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

// TestDeliveryServerMatchesRegisteredOperationCapabilities keeps each wire
// operation coupled only to the one handler method it invokes while still
// proving that the production Server covers the complete catalog. A monolithic
// Service interface would make every focused consumer and test fake depend on
// all operations merely to call one.
func TestDeliveryServerMatchesRegisteredOperationCapabilities(t *testing.T) {
	root := moduleRoot(t)
	operationDir := filepath.Join(root, "internal", "delivery", "operation")

	handlers := make(map[string]int)
	registrationCount := 0
	factories := map[string]struct{}{
		"Query": {}, "Command": {}, "CommandAck": {},
		"Subscription": {}, "RunSubscription": {}, "RunStreamCommand": {},
	}
	walkErr := filepath.WalkDir(operationDir, func(path string, entry fs.DirEntry, err error) error {
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
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			factory, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if _, registered := factories[factory.Name]; !registered {
				return true
			}
			registrationCount++
			if len(call.Args) == 0 {
				t.Errorf("%s registration has no typed handler", factory.Name)
				return true
			}
			literal, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
			if !ok || len(literal.Type.Params.List) == 0 {
				t.Errorf("%s registration must end in a typed handler closure", factory.Name)
				return true
			}
			capability, ok := literal.Type.Params.List[0].Type.(*ast.InterfaceType)
			if !ok || len(capability.Methods.List) != 1 || len(capability.Methods.List[0].Names) != 1 {
				t.Errorf("%s registration handler must declare exactly one method capability", factory.Name)
				return true
			}
			handlers[capability.Methods.List[0].Names[0].Name]++
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("inspect operation registrations: %v", walkErr)
	}

	registered := operation.Contract().Metas()
	if registrationCount != len(registered) {
		t.Errorf("operation registrations = %d, catalog methods = %d", registrationCount, len(registered))
	}
	if len(handlers) != registrationCount {
		t.Errorf("unique operation handler capabilities = %d, registrations = %d", len(handlers), registrationCount)
	}
	serverType := reflect.TypeFor[*deliveryserver.Server]()
	for name, count := range handlers {
		if count != 1 {
			t.Errorf("handler method %s is declared by %d operation capabilities, want one", name, count)
		}
		if _, exists := serverType.MethodByName(name); !exists {
			t.Errorf("registered handler method %s is absent from delivery Server", name)
		}
	}
	for method := range serverType.Methods() {
		if method.Name == "Close" {
			continue
		}
		if handlers[method.Name] == 0 {
			t.Errorf("delivery Server exports unregistered handler method %s", method.Name)
		}
	}
}

// TestCoreRunStateUsesBehaviorOwners prevents the two long-lived orchestration
// objects from regaining raw lock/channel/map state or the process-local Segment
// mechanisms that P113 moved behind invariant-owning components.
func TestCoreRunStateUsesBehaviorOwners(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		path       string
		structName string
		forbidden  map[string]string
		prefixes   map[string]string
	}{
		{
			path:       filepath.Join(root, "internal", "application", "runs", "coordinator.go"),
			structName: "Coordinator",
			forbidden: map[string]string{
				"taskgroup.Group":         "Segment task admission and join belong to segmentLifecycle",
				"registry":                "live Segment addressability belongs to segmentLifecycle",
				"sessionRunChanges":       "post-commit wakeups belong to runPublications",
				"ExecutionObserver":       "executor observation belongs to segmentLifecycle",
				"ExecutionReleaser":       "executor teardown belongs to segmentLifecycle",
				"SegmentFinalizer":        "terminal maintenance belongs to segmentLifecycle",
				"EventCommitter":          "write-before-publish ordering belongs to runPublications",
				"TreeBarrierCommitter":    "tree barrier publication belongs to runPublications",
				"WorkspaceChangeNotifier": "post-commit workspace notification belongs to runPublications",
			},
		},
		{
			path:       filepath.Join(root, "internal", "application", "runs", "execution_handoff.go"),
			structName: "claimedResumeAttempt",
			forbidden: map[string]string{
				"ExecutionReleaser": "staged execution cleanup belongs to stagedExecutionHandoff",
			},
		},
		{
			path:       filepath.Join(root, "internal", "application", "runs", "recovery.go"),
			structName: "Recovery",
			forbidden: map[string]string{
				"[]func()":            "Session claim release belongs to recoverySessionClaims",
				"map[string]struct{}": "claimed Session identity belongs to recoverySessionClaims",
			},
		},
		{
			path:       filepath.Join(root, "internal", "adapter", "agentexec", "interaction_session.go"),
			structName: "interactionSession",
			forbidden: map[string]string{
				"sync.Mutex":         "shared state requires a named invariant owner",
				"sync.Once":          "one-shot transitions belong to interactionLifetime",
				"sync.WaitGroup":     "goroutine join belongs to interactionLifetime",
				"context.Context":    "the lifecycle root belongs to interactionLifetime",
				"context.CancelFunc": "lifecycle cancellation belongs to interactionLifetime",
			},
			prefixes: map[string]string{
				"map[":  "shared maps require a named invariant owner",
				"chan ": "channels belong to interactionLifetime",
			},
		},
	}
	for _, check := range checks {
		for field, fieldType := range namedStructDirectFieldTypes(t, check.path, check.structName) {
			if reason := check.forbidden[fieldType]; reason != "" {
				t.Errorf("%s.%s directly stores %s; %s", check.structName, field, fieldType, reason)
			}
			for prefix, reason := range check.prefixes {
				if strings.HasPrefix(fieldType, prefix) {
					t.Errorf("%s.%s directly stores %s; %s", check.structName, field, fieldType, reason)
				}
			}
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
	path := filepath.Join(root, "internal", "domain", "transcript", "item.go")
	if got := namedStructFieldTypeOptional(t, path, "ItemIdentity", "OccurredAt"); got != "time.Time" {
		t.Errorf("transcript.ItemIdentity.OccurredAt = %q, want time.Time", got)
	}
	if got := namedStructFieldTypeOptional(t, path, "ItemIdentity", "CreatedAt"); got != "" {
		t.Errorf("transcript.ItemIdentity.CreatedAt = %q; variant-specific time belongs to Delivery", got)
	}
}

// TestTranscriptItemHasNoExternalMutationSurface prevents the closed Item
// union from becoming an exported field bag again. Technical codecs may use
// ItemSnapshot, but every production Item transition must enter through a
// semantic constructor or ToolCall behavior.
func TestTranscriptItemHasNoExternalMutationSurface(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "domain", "transcript", "item.go")
	if fields := namedStructExportedFields(t, path, "Item"); len(fields) != 0 {
		t.Errorf("transcript.Item exports mutable fields %v; use semantic behavior and accessors", fields)
	}
}

// TestTranscriptItemSnapshotStaysAtTechnicalBoundaries keeps RestoreItem and
// ItemSnapshot from becoming a convenient second mutation API in Application
// or ordinary adapters. Only strict persistence/archive codecs and test support
// may translate the complete technical representation.
func TestTranscriptItemSnapshotStaysAtTechnicalBoundaries(t *testing.T) {
	root := moduleRoot(t)
	allowed := map[string]struct{}{
		"internal/application/sessions/portable_snapshot.go":   {},
		"internal/application/sessions/snapshot_validation.go": {},
		"internal/delivery/server/artifact_decode.go":          {},
		"internal/infra/sqlite/transcript.go":                  {},
		"internal/infra/sqlite/transcript_codec.go":            {},
		"internal/testsupport/itemfixture/item.go":             {},
	}
	walkErr := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, accepted := allowed[filepath.ToSlash(relative)]; accepted {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := make(map[string]struct{})
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != "github.com/Tangerg/lynx/app/runtime/internal/domain/transcript" {
				continue
			}
			alias := "transcript"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			aliases[alias] = struct{}{}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "ItemSnapshot" && selector.Sel.Name != "RestoreItem") {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, imported := aliases[identifier.Name]; imported {
				t.Errorf("%s uses transcript.%s outside a strict technical codec", relative, selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan transcript Item snapshot boundary: %v", walkErr)
	}
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
		filepath.Join(root, "internal", "domain", "transcript"),
		filepath.Join(root, "internal", "domain", "interrupt"),
	}
	for _, dir := range dirs {
		walkErr := walkProductionGoFiles(dir, func(path string, file *ast.File) error {
			assertCanonicalExecutionRecordSource(t, root, path, file)
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
}

func assertCanonicalExecutionRecordSource(t *testing.T, root, path string, file *ast.File) {
	t.Helper()
	relative, _ := filepath.Rel(root, path)
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, `"`) == "encoding/json" {
			t.Errorf("%s: canonical execution records must not depend on JSON", relative)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		field, isField := node.(*ast.Field)
		if !isField {
			return true
		}
		for _, name := range field.Names {
			switch name.Name {
			case "Blob", "Payload", "JSON":
				t.Errorf(
					"%s: canonical execution field %s reintroduces an opaque persistence payload",
					relative,
					name.Name,
				)
			}
		}
		return true
	})
}

// TestRuntimeInterruptValuesStayWireFree keeps the application interrupt plan
// and the domain resume decision free of Agent Framework pending-input and
// Signal wire. The interactioninput ACL owns that translation; these values
// retain only product vocabulary.
func TestRuntimeInterruptValuesStayWireFree(t *testing.T) {
	root := moduleRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "application", "runs", "interrupt_contract.go"),
		filepath.Join(root, "internal", "domain", "interrupt", "resolution.go"),
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
		{filepath.Join(root, "internal", "domain", "interrupt", "resolution.go"), "Resolution"},
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
		{filepath.Join(root, "internal", "application", "runs", "journal.go"), "journal"},
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

var crossRingCapabilityImports = []string{
	"github.com/Tangerg/lynx/app/runtime/internal/completion",
	"github.com/Tangerg/lynx/app/runtime/internal/httporigin",
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency",
}

// domainPkg is the bounded-context ring. Prefix-matched.
const domainPkg = "github.com/Tangerg/lynx/app/runtime/internal/domain"

// protocolPkg is the wire-type package; it must stay pure wire (no domain /
// application import) so protocol types never leak inward (§16 rule 10).
const protocolPkg = "github.com/Tangerg/lynx/app/runtime/protocol"

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

// forbidTestImports applies a selected boundary to test fixtures as well. Most
// dependency rules intentionally ignore tests, but a Delivery fixture that
// imports Agent execution handles would teach the outer boundary to construct
// the implementation it is meant to drive only through application ports.
func forbidTestImports(t *testing.T, dir string, banned []string) {
	t.Helper()
	root := moduleRoot(t)
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			pathValue := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range banned {
				if pathValue == forbidden || strings.HasPrefix(pathValue, forbidden+"/") {
					relative, _ := filepath.Rel(root, path)
					t.Errorf("%s test fixture must not import %q", relative, pathValue)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s test imports: %v", dir, walkErr)
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
	walkErr := walkProductionGoFiles(dir, func(path string, file *ast.File) error {
		for _, name := range topLevelDeclarationNames(file) {
			reason, forbidden := banned[name]
			if !forbidden {
				continue
			}
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s: removed ownership seam %s returned; %s", relative, name, reason)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
}

func topLevelDeclarationNames(file *ast.File) []string {
	var names []string
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			names = append(names, typed.Name.Name)
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch named := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, named.Name.Name)
				case *ast.ValueSpec:
					for _, name := range named.Names {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	return names
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

func namedStructDirectFieldTypes(t *testing.T, path, structName string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			named, ok := specification.(*ast.TypeSpec)
			if !ok || named.Name.Name != structName {
				continue
			}
			value, ok := named.Type.(*ast.StructType)
			if !ok || value.Fields == nil {
				t.Fatalf("%s: %s is not a struct", path, structName)
			}
			fields := make(map[string]string)
			for _, field := range value.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = exprString(field.Type)
				}
			}
			return fields
		}
	}
	t.Fatalf("%s: type %s not found", path, structName)
	return nil
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

// namedStructExportedFields returns the exported fields declared directly on a
// named struct. Embedded exported fields count because they expose the same
// external mutation surface.
func namedStructExportedFields(t *testing.T, path, structName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			named, ok := specification.(*ast.TypeSpec)
			if !ok || named.Name.Name != structName {
				continue
			}
			value, ok := named.Type.(*ast.StructType)
			if !ok || value.Fields == nil {
				t.Fatalf("%s: %s is not a struct", path, structName)
			}
			var exported []string
			for _, field := range value.Fields.List {
				if len(field.Names) == 0 {
					name := exprString(field.Type)
					if ast.IsExported(strings.TrimPrefix(name, "*")) {
						exported = append(exported, name)
					}
					continue
				}
				for _, name := range field.Names {
					if name.IsExported() {
						exported = append(exported, name.Name)
					}
				}
			}
			return exported
		}
	}
	t.Fatalf("%s: type %s not found", path, structName)
	return nil
}

const (
	ringComposition = "composition"
	ringDelivery    = "delivery"
	ringAdapter     = "adapter"
	ringApplication = "application"
	ringInfra       = "infra"
	ringDomain      = "domain"
)

// layerOf classifies a module-relative package dir (e.g. "internal/infra/sqlite")
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
		// Infra is a reusable technical mechanism. I/O consumer ports belong to
		// Application and are translated by Adapter, so Infra never reaches either.
		return to != ringDomain && to != ringInfra
	case ringAdapter:
		// Adapters implement domain/application ports and wrap infra; they must
		// never reach up into delivery or the composition root (the latter would
		// let assembly logic hide inside a capability adapter).
		return to == ringDelivery || to == ringComposition
	case ringDelivery:
		// Delivery drives Application use cases and projects domain values. It
		// never reaches a concrete Adapter, raw Infra, or the composition root.
		return to == ringAdapter || to == ringInfra || to == ringComposition
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

// TestSystemInvariantCatalogStaysOutOfRuntimeRings keeps descriptive contract
// metadata out of production packages. The actual transaction behavior remains
// in Application; only contractgen names the published catalog.
func TestSystemInvariantCatalogStaysOutOfRuntimeRings(t *testing.T) {
	root := moduleRoot(t)
	forbidTopLevelNames(t, filepath.Join(root, "internal"), map[string]string{
		"SystemInvariantSpec": "a cross-resource invariant is named by the ring that owns its transaction",
		"TransactionBoundary": "descriptive contract metadata belongs outside the production graph",
	})
	// The generated catalog must remain actionable. This validation is
	// fitness-test behavior rather than a production Runtime dependency.
	specs := readSystemInvariantSpecs(t)
	if len(specs) == 0 {
		t.Fatal("no system invariant is declared; the manifest gate would pass vacuously")
	}
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		switch {
		case spec.Key == "":
			t.Error("system invariant key is required")
		case seen[spec.Key]:
			t.Errorf("system invariant %q is declared twice", spec.Key)
		case spec.Why == "":
			t.Errorf("system invariant %q does not explain what it protects", spec.Key)
		case len(spec.Boundaries) == 0:
			t.Errorf("system invariant %q has no responsible transaction", spec.Key)
		}
		seen[spec.Key] = true
	}
}
