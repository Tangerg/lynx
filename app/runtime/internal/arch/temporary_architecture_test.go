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

const oldAgentModulePath = "github.com/Tangerg/lynx/agent"

type temporaryImportAllowance struct {
	importCount int
	owner       string
	deletePhase string
}

// temporaryOldAgentImports is deliberately file-exact. A directory prefix
// would let the old framework spread while still reporting a green build.
// Counts are exact as well: every removed import shrinks this ledger in the same
// batch, and every added import fails instead of consuming an invisible budget.
var temporaryOldAgentImports = map[string]temporaryImportAllowance{
	"internal/adapter/agentexec/agent.go":                                      {2, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/build_identity_test.go":                        {1, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/chat_pipeline.go":                              {3, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/chat_pipeline_test.go":                         {1, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/checkpoint_restore.go":                         {1, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/child_execution.go":                            {2, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/config.go":                                     {1, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/deferred_manifest_test.go":                     {2, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/engine.go":                                     {2, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/engine_accounting_test.go":                     {1, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/engine_test.go":                                {2, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/observer.go":                                   {2, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/observer_test.go":                              {2, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/process_tree_codec.go":                         {1, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/process_tree_codec_test.go":                    {1, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/stub_model_test.go":                            {2, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/suspension/suspension.go":                      {1, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/tool_decorator.go":                             {1, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/turn/child_projection_test.go":                 {2, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/doomloop_test.go":                         {2, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/engine_fixtures_test.go":                  {2, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/engine_test.go":                           {2, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/lifecycle_hooks_test.go":                  {1, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/observer.go":                              {2, "adapter/agentexec legacy lifecycle", "P8"},
	"internal/adapter/agentexec/turn/policy_test.go":                           {2, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/rehydrate.go":                             {1, "adapter/agentexec legacy lifecycle", "P8"},
	"internal/adapter/agentexec/turn/shutdown_test.go":                         {1, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/subagent_lifecycle.go":                    {2, "adapter/agentexec legacy lifecycle", "P8"},
	"internal/adapter/agentexec/turn/terminal.go":                              {2, "adapter/agentexec legacy lifecycle", "P8"},
	"internal/adapter/agentexec/turn/terminal_test.go":                         {1, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turn/turn.go":                                  {1, "adapter/agentexec legacy lifecycle", "P8"},
	"internal/adapter/agentexec/turn/turn_control_subtree_test.go":             {1, "adapter/agentexec legacy lifecycle fixtures", "P8"},
	"internal/adapter/agentexec/turnloop.go":                                   {4, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/turnprocess.go":                                {3, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/turnprocess_test.go":                           {1, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/turnrun.go":                                    {2, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/turnrun_start_test.go":                         {3, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/agentexec/usage.go":                                      {5, "adapter/agentexec legacy executor", "P8"},
	"internal/adapter/agentexec/usage_test.go":                                 {2, "adapter/agentexec legacy executor fixtures", "P8"},
	"internal/adapter/runsegment/effects_sqlite_test.go":                       {2, "adapter/runsegment legacy effect fixtures", "P8"},
	"internal/adapter/runsegment/effects_waiting_cancellation_failure_test.go": {1, "adapter/runsegment legacy effect fixtures", "P8"},
	"internal/adapter/runsegment/effects_waiting_cancellation_restart_test.go": {1, "adapter/runsegment legacy effect fixtures", "P8"},
	"internal/adapter/toolset/delegation/delegation.go":                        {2, "adapter/toolset legacy framework tool bridge", "P5"},
	"internal/adapter/toolset/discovery/discovery.go":                          {1, "adapter/toolset legacy framework tool bridge", "P5"},
	"internal/adapter/toolset/discovery/discovery_test.go":                     {1, "adapter/toolset legacy framework tool fixtures", "P5"},
	"internal/adapter/toolset/exposure_test.go":                                {1, "adapter/toolset legacy framework tool fixtures", "P5"},
	"internal/adapter/toolset/pathlock.go":                                     {1, "adapter/toolset legacy framework tool bridge", "P5"},
	"internal/adapter/toolset/pathlock_test.go":                                {1, "adapter/toolset legacy framework tool fixtures", "P5"},
	"internal/adapter/toolset/resolver.go":                                     {1, "adapter/toolset legacy framework tool bridge", "P5"},
	"internal/adapter/toolset/resolver_discovery_test.go":                      {1, "adapter/toolset legacy framework tool fixtures", "P5"},
	"internal/adapter/toolset/resolver_roles_test.go":                          {1, "adapter/toolset legacy framework tool fixtures", "P5"},
	"internal/bootstrap/assemble_test.go":                                      {2, "bootstrap legacy executor wiring fixtures", "P8"},
	"internal/bootstrap/session_writesets_test.go":                             {2, "bootstrap legacy lifecycle fixtures", "P8"},
}

func TestTemporaryOldAgentImportsAreExact(t *testing.T) {
	root := moduleRoot(t)
	actual := make(map[string]int)
	err := walkGoFiles(root, func(path string, file *ast.File) {
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath != oldAgentModulePath && !strings.HasPrefix(importPath, oldAgentModulePath+"/") {
				continue
			}
			relativePath, _ := filepath.Rel(root, path)
			actual[filepath.ToSlash(relativePath)]++
		}
	})
	if err != nil {
		t.Fatalf("scan old Agent imports: %v", err)
	}

	for path, count := range actual {
		allowance, allowed := temporaryOldAgentImports[path]
		if !allowed {
			t.Errorf("old Agent import has no temporary owner: %s (%d imports)", path, count)
			continue
		}
		if count != allowance.importCount {
			t.Errorf("%s old Agent import count = %d, ledger = %d; shrink the ledger with every removal and reject every addition", path, count, allowance.importCount)
		}
		if allowance.owner == "" || allowance.deletePhase == "" {
			t.Errorf("%s temporary import lacks owner or deletion phase", path)
		}
	}
	for path := range temporaryOldAgentImports {
		if actual[path] == 0 {
			t.Errorf("stale old Agent exception %s: remove it in the same batch as the import", path)
		}
	}
}

type temporaryDomainPort struct {
	owner       string
	deletePhase string
}

var temporaryDomainIOPorts = map[string]temporaryDomainPort{}

func TestTemporaryDomainIOPortsAreExact(t *testing.T) {
	root := moduleRoot(t)
	domainRoot := filepath.Join(root, "internal", "domain")
	for name, port := range temporaryDomainIOPorts {
		if port.owner == "" || port.deletePhase == "" {
			t.Errorf("Domain I/O port %s lacks an owner or deletion phase", name)
		}
	}
	actual := make(map[string]struct{})
	err := walkGoFiles(domainRoot, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				named, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				contract, ok := named.Type.(*ast.InterfaceType)
				if !ok || !interfaceUsesContext(contract) {
					continue
				}
				relativePath, _ := filepath.Rel(root, path)
				actual[filepath.ToSlash(relativePath)+":"+named.Name.Name] = struct{}{}
			}
		}
	})
	if err != nil {
		t.Fatalf("scan Domain I/O ports: %v", err)
	}

	compareTemporarySet(t, "Domain I/O port", actual, temporaryDomainIOPorts)
}

func interfaceUsesContext(contract *ast.InterfaceType) bool {
	if contract.Methods == nil {
		return false
	}
	for _, method := range contract.Methods.List {
		usesContext := false
		ast.Inspect(method.Type, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Context" {
				return true
			}
			packageName, ok := selector.X.(*ast.Ident)
			if ok && packageName.Name == "context" {
				usesContext = true
			}
			return true
		})
		if usesContext {
			return true
		}
	}
	return false
}

type temporaryComponent struct {
	owner       string
	deletePhase string
}

var temporaryComponents = map[string]temporaryComponent{
	"completion":   {"internal/component/completion", "P9"},
	"httporigin":   {"internal/component/httporigin", "P9"},
	"idempotency":  {"internal/component/idempotency", "P9"},
	"keyset":       {"internal/component/keyset", "P9"},
	"pathidentity": {"internal/component/pathidentity", "P9"},
	"replaycursor": {"internal/component/replaycursor", "P9"},
	"secretmask":   {"internal/component/secretmask", "P9"},
	"shutdown":     {"internal/component/shutdown", "P9"},
	"signal":       {"internal/component/signal", "P9"},
	"taskgroup":    {"internal/component/taskgroup", "P9"},
}

func TestTemporaryComponentPackagesAreExact(t *testing.T) {
	root := moduleRoot(t)
	componentRoot := filepath.Join(root, "internal", "component")
	for name, component := range temporaryComponents {
		if component.owner == "" || component.deletePhase == "" {
			t.Errorf("component package %s lacks an owner or deletion phase", name)
		}
	}
	entries, err := os.ReadDir(componentRoot)
	if err != nil {
		t.Fatalf("read component umbrella: %v", err)
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			actual[entry.Name()] = struct{}{}
		}
	}
	compareTemporarySet(t, "component package", actual, temporaryComponents)
}

func TestTemporaryPrivateSnapshotDecoderIsExact(t *testing.T) {
	root := moduleRoot(t)
	want := filepath.ToSlash("internal/adapter/agentexec/process_tree_codec.go")
	var actual []string
	err := walkGoFiles(root, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") || !importsPackage(file, oldAgentModulePath+"/core") {
			return
		}
		if !callsSelector(file, "json", "Unmarshal") {
			return
		}
		relativePath, _ := filepath.Rel(root, path)
		actual = append(actual, filepath.ToSlash(relativePath))
	})
	if err != nil {
		t.Fatalf("scan private snapshot decoders: %v", err)
	}
	if len(actual) != 1 || actual[0] != want {
		t.Fatalf("old private snapshot decoder owners = %v, want sole P8 exception %s", actual, want)
	}
}

// TestTemporaryTurnProcessOwnerIsExact keeps the old lifecycle from being
// copied while Agent2 is introduced in a parallel harness. P8 removes this sole
// declaration when production switches atomically.
func TestTemporaryTurnProcessOwnerIsExact(t *testing.T) {
	root := moduleRoot(t)
	want := filepath.ToSlash("internal/adapter/agentexec/turnprocess.go")
	var owners []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				named, ok := specification.(*ast.TypeSpec)
				if ok && named.Name.Name == "TurnProcess" {
					relativePath, _ := filepath.Rel(root, path)
					owners = append(owners, filepath.ToSlash(relativePath))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan TurnProcess owners: %v", err)
	}
	if len(owners) != 1 || owners[0] != want {
		t.Fatalf("TurnProcess owners = %v, want sole P8 exception %s", owners, want)
	}
}

func walkGoFiles(root string, visit func(path string, file *ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	})
}

func importsPackage(file *ast.File, packagePath string) bool {
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, `"`) == packagePath {
			return true
		}
	}
	return false
}

func callsSelector(file *ast.File, packageName, functionName string) bool {
	called := false
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != functionName {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == packageName {
			called = true
		}
		return true
	})
	return called
}

func compareTemporarySet[T any](
	t *testing.T,
	name string,
	actual map[string]struct{},
	ledger map[string]T,
) {
	t.Helper()
	for item := range actual {
		if _, allowed := ledger[item]; !allowed {
			t.Errorf("%s has no temporary owner and deletion phase: %s", name, item)
		}
	}
	for item := range ledger {
		if _, exists := actual[item]; !exists {
			t.Errorf("stale %s exception: %s", name, item)
		}
	}
}
