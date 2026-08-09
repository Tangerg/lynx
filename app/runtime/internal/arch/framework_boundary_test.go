package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const oldAgentModulePath = "github.com/Tangerg/lynx/agent"

// TestOldAgentModuleIsAbsent prevents the superseded framework from returning
// after the production execution cutover. Runtime has one framework boundary:
// Agent2 through adapter/agentexec.
func TestOldAgentModuleIsAbsent(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || (strings.HasPrefix(entry.Name(), ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath != oldAgentModulePath && !strings.HasPrefix(importPath, oldAgentModulePath+"/") {
				continue
			}
			relativePath, _ := filepath.Rel(root, path)
			t.Errorf("old Agent import remains after production cutover: %s", filepath.ToSlash(relativePath))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan old Agent imports: %v", err)
	}
}

// TestDomainHasNoContextIOPorts keeps external interaction out of the Domain
// ring. Domain may own pure strategy contracts; context-bearing I/O contracts
// belong to their Application consumers.
func TestDomainHasNoContextIOPorts(t *testing.T) {
	root := moduleRoot(t)
	domainRoot := filepath.Join(root, "internal", "domain")
	err := filepath.WalkDir(domainRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
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
				t.Errorf("Domain context I/O port is forbidden: %s:%s", filepath.ToSlash(relativePath), named.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Domain I/O ports: %v", err)
	}
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

// TestSQLiteDoesNotDiscardRowsAffectedErrors protects compare-and-swap and
// mutation outcomes from treating a driver failure as a successful zero count.
func TestSQLiteDoesNotDiscardRowsAffectedErrors(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal", "infra", "storage", "sqlite")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			callsRowsAffected := false
			for _, expression := range assignment.Rhs {
				call, ok := expression.(*ast.CallExpr)
				if !ok {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "RowsAffected" {
					callsRowsAffected = true
					break
				}
			}
			if !callsRowsAffected {
				return true
			}
			if len(assignment.Lhs) < 2 {
				return true
			}
			identifier, ok := assignment.Lhs[1].(*ast.Ident)
			if ok && identifier.Name == "_" {
				t.Errorf("%s discards a RowsAffected error", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan SQLite sources: %v", err)
	}
}

// TestExecutorCheckpointRemainsOpaqueOutsideExecutionAdapter prevents the
// application from reconstructing executor process topology. It may own only the aggregate
// root identity, opaque payload, and host metadata required for lifecycle
// policy.
func TestExecutorCheckpointRemainsOpaqueOutsideExecutionAdapter(t *testing.T) {
	root := moduleRoot(t)
	checkpointPath := filepath.Join(root, "internal", "application", "runs", "executor_checkpoint.go")
	checkpointFile, err := parser.ParseFile(token.NewFileSet(), checkpointPath, nil, 0)
	if err != nil {
		t.Fatalf("parse executor checkpoint: %v", err)
	}
	checkpoint := structFields(checkpointFile, "ExecutorCheckpoint")
	want := []string{"RootMemberID", "Payload", "BuildID", "Scope", "ModelSelection", "Limits", "Capabilities", "Usage"}
	if strings.Join(checkpoint, ",") != strings.Join(want, ",") {
		t.Fatalf("ExecutorCheckpoint fields = %v, want opaque application envelope %v", checkpoint, want)
	}

	storagePath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "executor_checkpoint.go")
	storageSource, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("read executor checkpoint store: %v", err)
	}
	for _, forbidden := range []string{
		"agent/core", "domain/session", "parent_process_id", "started_at", "ProcessSnapshot", "ProcessTreeState",
	} {
		if strings.Contains(string(storageSource), forbidden) {
			t.Errorf("executor checkpoint storage leaks %q", forbidden)
		}
	}
	if !strings.Contains(string(storageSource), "DisallowUnknownFields") {
		t.Error("durable interrupt storage accepts unknown legacy topology fields")
	}
}

// TestRunLimitsRemainTheSingleApplicationPolicy prevents the executor, durable
// hand-off, and public protocol from growing separate budget carriers again.
// The top-level cumulative token ceiling is deliberately distinct from the
// per-model-call GenerationParams.MaxTokens option.
func TestRunLimitsRemainTheSingleApplicationPolicy(t *testing.T) {
	root := moduleRoot(t)
	domainPath := filepath.Join(root, "internal", "domain", "run", "admission.go")
	domainFile, err := parser.ParseFile(token.NewFileSet(), domainPath, nil, 0)
	if err != nil {
		t.Fatalf("parse execution admission: %v", err)
	}
	wantLimits := []string{"MaxTotalTokens", "MaxSteps", "MaxBudgetUSD"}
	if fields := structFields(domainFile, "RunLimits"); strings.Join(fields, ",") != strings.Join(wantLimits, ",") {
		t.Fatalf("RunLimits fields = %v, want the single policy carrier %v", fields, wantLimits)
	}

	accountingRoot := filepath.Join(root, "internal", "domain", "accounting")
	err = filepath.WalkDir(accountingRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(source), "type Budget struct") {
			t.Errorf("%s recreates a second execution budget carrier", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan execution accounting: %v", err)
	}

	protocolPath := filepath.Join(root, "internal", "delivery", "protocol", "runs.go")
	protocolFile, err := parser.ParseFile(token.NewFileSet(), protocolPath, nil, 0)
	if err != nil {
		t.Fatalf("parse run protocol: %v", err)
	}
	startFields := structFields(protocolFile, "StartRunRequest")
	if !slices.Contains(startFields, "MaxTotalTokens") || slices.Contains(startFields, "MaxTokens") {
		t.Fatalf("StartRunRequest fields = %v, want cumulative MaxTotalTokens without ambiguous MaxTokens", startFields)
	}
	generationFields := structFields(protocolFile, "GenerationParams")
	if !slices.Contains(generationFields, "MaxTokens") || slices.Contains(generationFields, "MaxTotalTokens") {
		t.Fatalf("GenerationParams fields = %v, want per-call MaxTokens only", generationFields)
	}

	for _, carrier := range []struct {
		path string
		name string
	}{
		{path: filepath.Join("internal", "application", "runs", "commands.go"), name: "StartCommand"},
		{path: filepath.Join("internal", "application", "runs", "ports.go"), name: "RootExecutionStart"},
	} {
		path := filepath.Join(root, carrier.path)
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", carrier.path, parseErr)
		}
		fields := structFields(file, carrier.name)
		if !slices.Contains(fields, "Limits") {
			t.Errorf("%s fields = %v, want the complete RunLimits value", carrier.name, fields)
		}
		for _, duplicate := range []string{"MaxTotalTokens", "MaxSteps", "MaxCostUSD", "MaxBudgetUSD"} {
			if slices.Contains(fields, duplicate) {
				t.Errorf("%s recreates RunLimits dimension %s as a parallel field", carrier.name, duplicate)
			}
		}
	}

	for _, relative := range []string{
		filepath.Join("internal", "infra", "storage", "sqlite", "db.go"),
		filepath.Join("internal", "infra", "storage", "sqlite", "runs.go"),
	} {
		source, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			t.Fatalf("read %s: %v", relative, readErr)
		}
		if !strings.Contains(string(source), "max_total_tokens") || strings.Contains(string(source), "max_tokens") {
			t.Errorf("%s does not use the canonical max_total_tokens storage name", relative)
		}
	}
}

// TestWaitingCheckpointPersistenceBelongsToApplicationTransactions locks the
// ownership split: agentexec captures/restores opaque data, while runsegment
// atomically saves or removes it with the Pending and Run transitions that own
// the continuation.
func TestWaitingCheckpointPersistenceBelongsToApplicationTransactions(t *testing.T) {
	root := moduleRoot(t)
	assertAgentexecHasNoCheckpointWrites(t, root)

	commitPath := filepath.Join(root, "internal", "adapter", "runsegment", "effects_commit.go")
	commitFile, err := parser.ParseFile(token.NewFileSet(), commitPath, nil, 0)
	if err != nil {
		t.Fatalf("parse runsegment effects: %v", err)
	}
	barrier := methodBody(commitFile, "Effects", "CommitTreeBarrier")
	transaction, ok := calledClosure(barrier, "runInTx")
	if !ok {
		t.Fatal("CommitTreeBarrier does not execute one application transaction")
	}
	for _, required := range []string{"SaveCheckpoint", "openInterrupt", "applyCommit"} {
		if !selectorCallExists(transaction.Body, required) {
			t.Errorf("CommitTreeBarrier transaction does not own %s", required)
		}
	}
	terminal := methodBody(commitFile, "Effects", "CommitEvent")
	terminalTx, ok := calledClosure(terminal, "runInTx")
	if !ok || !selectorCallExists(terminalTx.Body, "DeleteCheckpoints") {
		t.Error("CommitEvent terminal transaction does not own executor checkpoint deletion")
	}

	waitingPath := filepath.Join(root, "internal", "adapter", "runsegment", "effects_waiting_cancellation.go")
	waitingFile, err := parser.ParseFile(token.NewFileSet(), waitingPath, nil, 0)
	if err != nil {
		t.Fatalf("parse waiting cancellation effects: %v", err)
	}
	waiting := methodBody(waitingFile, "Effects", "CommitWaitingSubtreeCancellation")
	waitingTx, ok := calledClosure(waiting, "runInTx")
	if !ok || !selectorCallExists(waitingTx.Body, "SaveCheckpoint") {
		t.Error("waiting subtree transaction does not own replacement checkpoint persistence")
	}
}

// TestBootRecoveryPolicyBelongsToApplication prevents SQLite from regaining
// executor callbacks or Run lifecycle decisions. Storage exposes facts, the
// Application derives one RecoveryCommit, and the driven adapter applies it.
func TestBootRecoveryPolicyBelongsToApplication(t *testing.T) {
	root := moduleRoot(t)
	sqlitePath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "recovery_projection.go")
	sqliteFile, err := parser.ParseFile(token.NewFileSet(), sqlitePath, nil, 0)
	if err != nil {
		t.Fatalf("parse SQLite recovery projection: %v", err)
	}
	for _, declaration := range sqliteFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if function.Name.Name != "ListNonTerminalRuns" {
			t.Errorf("SQLite recovery file owns policy function %s; want fact projection only", function.Name.Name)
		}
	}

	applicationPath := filepath.Join(root, "internal", "application", "runs", "recovery.go")
	applicationSource, err := os.ReadFile(applicationPath)
	if err != nil {
		t.Fatalf("read application recovery: %v", err)
	}
	for _, required := range []string{"type Recovery struct", "type RecoveryCommit struct", "CanResumeCheckpoint", "RecoverLost"} {
		if !strings.Contains(string(applicationSource), required) {
			t.Errorf("application recovery does not own %q", required)
		}
	}

	bootstrapPath := filepath.Join(root, "internal", "bootstrap", "assemble.go")
	bootstrapSource, err := os.ReadFile(bootstrapPath)
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	for _, required := range []string{"runs.NewRecovery", "bootRecovery.Reconcile"} {
		if !strings.Contains(string(bootstrapSource), required) {
			t.Errorf("bootstrap does not drive application recovery through %q", required)
		}
	}
	for _, forbidden := range []string{"ReconcileOrphans", "ResumableProcessValidator"} {
		if strings.Contains(string(bootstrapSource), forbidden) {
			t.Errorf("bootstrap restores obsolete recovery seam %q", forbidden)
		}
	}
}

// TestExecutionContextIsNeutralBetweenPeerAdapters prevents application-owned
// execution scope from being nested under agentexec again.
func TestExecutionContextIsNeutralBetweenPeerAdapters(t *testing.T) {
	root := moduleRoot(t)
	legacy := filepath.Join(root, "internal", "adapter", "agentexec", "turnctx")
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy agentexec-owned turn context still exists: %s", legacy)
	}
	neutral := filepath.Join(root, "internal", "adapter", "executionctx", "executionctx.go")
	if _, err := os.Stat(neutral); err != nil {
		t.Fatalf("neutral execution context is missing: %v", err)
	}
}

// TestApplicationExecutionPortsUseApplicationVocabulary locks the P3
// consumer-owned root seam. Later Agent2 vertical slices may deliberately
// revise this candidate, but they may not restore the old implementation-shaped
// facade or Framework Process vocabulary in Application.
func TestApplicationExecutionPortsUseApplicationVocabulary(t *testing.T) {
	root := moduleRoot(t)
	portsPath := filepath.Join(root, "internal", "application", "runs", "ports.go")
	portsFile, err := parser.ParseFile(token.NewFileSet(), portsPath, nil, 0)
	if err != nil {
		t.Fatalf("parse Run ports: %v", err)
	}

	wantMethods := map[string][]string{
		"RootExecutionStarter": {"ValidateRootStart", "StageRoot", "BeginRoot"},
		"ExecutionObserver":    {"Observe"},
		"ExecutionReleaser":    {"Release"},
	}
	for interfaceName, want := range wantMethods {
		got := interfaceMethods(portsFile, interfaceName)
		if !slices.Equal(got, want) {
			t.Errorf("%s methods = %v, want %v", interfaceName, got, want)
		}
	}

	applicationRoot := filepath.Join(root, "internal", "application")
	forbidden := []string{
		"ExecutionControl", "SegmentExecutor", "SessionLifecycle", "ExecutionCleanup",
		"PrepareStart", "ValidateStart",
		"CancelExecution", "StartExecution", "ExecutorSource", "ProcessSuspension",
		"RootProcessID", "ProcessID",
	}
	err = filepath.WalkDir(applicationRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, term := range forbidden {
			if strings.Contains(string(source), term) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s restores forbidden execution vocabulary %q", relative, term)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Application execution vocabulary: %v", err)
	}

	for _, relative := range []string{
		filepath.Join("internal", "infra", "storage", "sqlite", "db.go"),
		filepath.Join("internal", "infra", "storage", "sqlite", "executor_checkpoint.go"),
		filepath.Join("internal", "infra", "storage", "sqlite", "interrupt.go"),
	} {
		source, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			t.Fatalf("read %s: %v", relative, readErr)
		}
		for _, term := range []string{"root_process_id", "rootProcessId", `json:"processId"`} {
			if strings.Contains(string(source), term) {
				t.Errorf("%s restores forbidden persisted executor vocabulary %q", relative, term)
			}
		}
	}
}

// TestPendingStoresOnlyOpaqueExecutorBindings prevents the application continuation
// from persisting a second copy of executor parent/spawn topology. Run lineage
// is the durable product tree; live executor topology is validated only while
// routing events through the adapter boundary.
func TestPendingStoresOnlyOpaqueExecutorBindings(t *testing.T) {
	root := moduleRoot(t)
	interruptPath := filepath.Join(root, "internal", "application", "runs", "pending.go")
	interruptFile, err := parser.ParseFile(token.NewFileSet(), interruptPath, nil, 0)
	if err != nil {
		t.Fatalf("parse interrupt domain: %v", err)
	}
	want := []string{
		"RunID", "MemberID", "Lineage", "ModelSelection", "DrainedTools",
		"CommittedTools", "RunCreatedAt", "Metrics", "Limits",
	}
	if fields := structFields(interruptFile, "Continuation"); strings.Join(fields, ",") != strings.Join(want, ",") {
		t.Fatalf("Continuation fields = %v, want application facts plus one opaque executor binding %v", fields, want)
	}

	storagePath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "interrupt.go")
	storageSource, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("read interrupt storage: %v", err)
	}
	for _, forbidden := range []string{
		"ParentProcessID", "SpawnCallID", "parentProcessId", "spawnCallId",
		"parent_process_id", "spawn_call_id",
	} {
		if strings.Contains(string(storageSource), forbidden) {
			t.Errorf("durable interrupt storage persists Framework topology %q", forbidden)
		}
	}
}

// TestSubagentHooksUseApplicationRunIdentity keeps the user-facing lifecycle
// contract aligned with the product model. Executor process identities remain
// private routing values and cannot escape through hook JSON.
func TestSubagentHooksUseApplicationRunIdentity(t *testing.T) {
	root := moduleRoot(t)
	hookPath := filepath.Join(root, "internal", "domain", "hooks", "hooks.go")
	hookFile, err := parser.ParseFile(token.NewFileSet(), hookPath, nil, 0)
	if err != nil {
		t.Fatalf("parse hook domain: %v", err)
	}
	want := []string{"RunID", "ParentRunID", "Description", "Prompt", "Status", "Result", "Error"}
	if fields := structFields(hookFile, "SubagentInput"); strings.Join(fields, ",") != strings.Join(want, ",") {
		t.Fatalf("SubagentInput fields = %v, want application lifecycle identity %v", fields, want)
	}

	shellPath := filepath.Join(root, "internal", "adapter", "hooks", "shell.go")
	shellSource, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatalf("read hook shell adapter: %v", err)
	}
	for _, required := range []string{"json:\"runId\"", "json:\"parentRunId,omitempty\""} {
		if !strings.Contains(string(shellSource), required) {
			t.Errorf("hook JSON no longer exposes application identity marker %q", required)
		}
	}
	for _, forbidden := range []string{"processId", "parentProcessId", "ParentProcessID"} {
		if strings.Contains(string(shellSource), forbidden) {
			t.Errorf("hook JSON leaks Framework identity %q", forbidden)
		}
	}
}

// TestToolsetDoesNotDependOnAgentexec prevents peer adapters from recreating a
// consumer port under the execution adapter and then importing it backwards.
// Generic tool vocabulary belongs to domain/tool, concrete identities stay in
// toolset/catalog, and the HITL capability belongs to the runs consumer contract.
func TestToolsetDoesNotDependOnAgentexec(t *testing.T) {
	root := moduleRoot(t)
	legacy := filepath.Join(root, "internal", "adapter", "agentexec", "toolport")
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy agentexec-owned tool port still exists: %s", legacy)
	}

	toolsetRoot := filepath.Join(root, "internal", "adapter", "toolset")
	err := filepath.WalkDir(toolsetRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(source), "internal/adapter/agentexec/") {
			t.Errorf("peer toolset source %s imports agentexec", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan toolset source: %v", err)
	}
}

// TestAgentexecDoesNotOwnConcreteToolContracts keeps model-facing tool names,
// schemas, special gate policy, and tool-specific outcome projection in
// toolset. Agent execution resolves generic tools and translates their lifecycle
// through its consumer-owned interpreter port.
func TestAgentexecDoesNotOwnConcreteToolContracts(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "adapter", "agentexec")
	forbidden := []string{`"delegate_task"`, `"set_plan"`, `"exit_plan_mode"`, `"report_goal_outcome"`}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, name := range forbidden {
			if strings.Contains(string(source), name) {
				t.Errorf("agent execution source %s owns concrete tool name %s", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan agent execution source: %v", err)
	}
}

// TestRuntimeOwnedToolNamesComeFromCatalog prevents constructors from opening a
// second identity source beside toolset/catalog. Dynamically discovered MCP and
// A2A names remain values; only authored string literals in Name fields are
// forbidden here.
func TestRuntimeOwnedToolNamesComeFromCatalog(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "adapter", "toolset")
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") ||
			strings.HasPrefix(path, filepath.Join(dir, "catalog")+string(filepath.Separator)) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			pair, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok || key.Name != "Name" {
				return true
			}
			literal, authored := pair.Value.(*ast.BasicLit)
			if !authored || literal.Kind != token.STRING {
				return true
			}
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s authors Name %s; use toolset/catalog for a built-in identity", relative, literal.Value)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan toolset identities: %v", err)
	}
}

// TestFilesystemMutationVocabularyIsModelIndependent pins one precise model
// contract: every Run receives apply_patch, without model-id heuristics or a
// parallel edit/write family.
func TestFilesystemMutationVocabularyIsModelIndependent(t *testing.T) {
	root := moduleRoot(t)
	for _, relative := range []string{
		"internal/adapter/toolset/build.go",
		"internal/adapter/toolset/exposure.go",
		"internal/adapter/toolset/resolver.go",
		"internal/adapter/toolset/cwd_tools.go",
	} {
		path := filepath.Join(root, relative)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, forbidden := range []string{
			"useApplyPatch", "editWrite", "NewEditTool", "NewWriteTool",
			"ModelSelection", "DefaultModel", "gpt-", "grok", "claude", "kimi",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains mutation-dialect branch %q", relative, forbidden)
			}
		}
	}
}

// TestSegmentPumpKeepsOneGoroutineOwner prevents event routing, reducers, and
// terminal synthesis from being fanned out after they were consolidated under
// one concrete state owner. Parallelism belongs upstream in executor processes;
// Run projection order remains serial here.
func TestSegmentPumpKeepsOneGoroutineOwner(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "internal", "application", "runs", "pump.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse Run pump: %v", err)
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || receiverName(function.Recv) != "segmentPump" {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if _, startsWorker := node.(*ast.GoStmt); startsWorker {
				t.Errorf("segmentPump.%s starts a second goroutine", function.Name.Name)
			}
			return true
		})
	}
	if !found {
		t.Fatal("Run pump no longer has a concrete segmentPump state owner")
	}
}

// TestInnerRingCommentsDoNotNameOuterArchitecture prevents documentation from
// rebuilding a reverse dependency in the reader's mental model after imports
// have been cleaned up. Comments describe owned semantics and inward ports,
// never a caller or composition package that happens to consume them today.
func TestInnerRingCommentsDoNotNameOuterArchitecture(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal")
	runtimeProtocolReference := regexp.MustCompile(`(?i)API\.md|AUX_API|TRANSPORT\.md|\b(workspace\.listFiles|sessions\.(rollback|fork|import)|runs\.(start|cancel|resume)|providers\.(update|test)|models\.(list|setUtilityRole|getUtilityRole|setEmbeddingRole|getEmbeddingRole)|goals\.start|mcp\.tools\.list)\b|matches? the wire|maps? to the wire|transport maps|transport-shape|protocol boundary owns the wire|runtime composition layer|runtime server process`)
	checks := []struct {
		ring      string
		forbidden *regexp.Regexp
	}{
		{
			ring:      "domain",
			forbidden: regexp.MustCompile(`(?i)\b(application|adapters?|delivery|infrastructure|infra|bootstrap|frontend|desktop|tui|cli|chat engine)\b|@[a-z]`),
		},
		{
			ring:      "application",
			forbidden: regexp.MustCompile(`(?i)\b(adapters?|delivery|infrastructure|infra|bootstrap|frontend|desktop|tui|cli)\b|composition[ -]root|transport method|@[a-z]`),
		},
		{
			ring:      "adapter",
			forbidden: regexp.MustCompile(`(?i)\b(delivery|bootstrap|frontend|desktop|tui|cli)\b|composition[ -]root|server's commandResult|@[a-z]`),
		},
		{
			ring:      "infra",
			forbidden: regexp.MustCompile(`(?i)delivery[/ -](layer|protocol|server)|\b(bootstrap|frontend|desktop|tui|cli)\b|composition[ -]root|internal/adapter/`),
		},
		{
			ring:      "config",
			forbidden: regexp.MustCompile(`(?i)\b(delivery|bootstrap|frontend|desktop|tui|cli)\b|composition[ -]root`),
		},
	}
	for _, check := range checks {
		t.Run(check.ring, func(t *testing.T) {
			dir := filepath.Join(root, check.ring)
			err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
				if err != nil {
					return err
				}
				for _, group := range file.Comments {
					if leaked := check.forbidden.FindString(group.Text()); leaked != "" {
						t.Errorf("%s comment names outer architecture %q", path, leaked)
					}
					if leaked := runtimeProtocolReference.FindString(group.Text()); leaked != "" {
						t.Errorf("%s comment names Runtime protocol surface %q", path, leaked)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s comments: %v", check.ring, err)
			}
		})
	}
}

// TestStackExposesTheIdempotencyConsumerPort keeps concrete storage ownership
// inside construction. The assembled discovery surface crosses a package
// boundary, so it exposes exactly the behavior its HTTP consumer requires.
func TestStackExposesTheIdempotencyConsumerPort(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "internal", "bootstrap", "assemble.go")
	if got := namedStructFieldType(t, path, "Stack", "IdempotencyStore"); got != "idempotency.Store" {
		t.Fatalf("Stack.IdempotencyStore type = %s, want consumer port idempotency.Store", got)
	}
}

// TestExecutorCheckpointBindingIsValidatedAtEveryBoundary locks the
// cross-aggregate invariant that a valid checkpoint and a valid Pending are
// insufficient unless they describe the same root, Session, model selection, goal
// lease, and restore workspace.
func TestExecutorCheckpointBindingIsValidatedAtEveryBoundary(t *testing.T) {
	root := moduleRoot(t)
	checks := map[string][]string{
		filepath.Join("internal", "application", "runs", "execution_fact.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "application", "runs", "waiting_cancellation.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "application", "runs", "commit.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "application", "runs", "waiting_cancellation_commit.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "adapter", "runsegment", "effects_commit.go"): {
			"DeleteCheckpoints(ctx, commit.SessionID",
		},
		filepath.Join("internal", "application", "runs", "recovery_validation.go"): {
			"ExecutorCheckpointExpectation", "GoalLeaseID", "sess.CWD", "sess.Isolated",
		},
		filepath.Join("internal", "adapter", "agentexec", "interaction_recovery_probe.go"): {
			"ValidateFor", "GoalLeaseID", "Capabilities",
		},
		filepath.Join("internal", "adapter", "persistence", "session_stores.go"): {
			"DeleteCheckpoints(ctx, plan.SessionID",
			"DeleteCheckpoints(ctx, root.SessionID",
		},
		filepath.Join("internal", "infra", "storage", "sqlite", "executor_checkpoint.go"): {
			"deleteOwnedCheckpoint(ctx, sessionID",
			"case owner != checkpoint.Scope.SessionID",
			"case buildID != checkpoint.BuildID",
			"case policy != string(encodedPolicy)",
			"ValidateAdvanceFrom(storedUsage)",
		},
	}
	for relative, required := range checks {
		path := filepath.Join(root, relative)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, marker := range required {
			if !strings.Contains(string(source), marker) {
				t.Errorf("%s no longer validates checkpoint binding marker %q", relative, marker)
			}
		}
	}
}

// TestPendingAndRecoveryMutationsCarryTheirOwners prevents root identities from
// becoming ambient mutation authority. An open barrier cannot be overwritten;
// only its exact resuming owner may advance to the next quiescent barrier. Every
// destructive operation names its Session owner, and recovery validates its
// complete write-set before persistence begins.
func TestPendingAndRecoveryMutationsCarryTheirOwners(t *testing.T) {
	root := moduleRoot(t)
	interruptPath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "interrupt.go")
	interruptSource, err := os.ReadFile(interruptPath)
	if err != nil {
		t.Fatalf("read interrupt store: %v", err)
	}
	for _, required := range []string{
		"ON CONFLICT(root_run_id) DO UPDATE SET",
		"WHERE interrupts.state = 'resuming'",
		"AND interrupts.session_id = excluded.session_id",
		"AND interrupts.executor_id = excluded.executor_id",
		"AND interrupts.root_member_id = excluded.root_member_id",
	} {
		if !strings.Contains(string(interruptSource), required) {
			t.Errorf("interrupt store no longer guards barrier advance with %q", required)
		}
	}
	persistencePath := filepath.Join(root, "internal", "adapter", "persistence", "interrupts.go")
	persistenceSource, err := os.ReadFile(persistencePath)
	if err != nil {
		t.Fatalf("read interrupt persistence adapter: %v", err)
	}
	if !strings.Contains(string(persistenceSource), "Open(ctx context.Context, pending runs.Pending)") {
		t.Error("interrupt persistence adapter no longer accepts the Application-owned Pending value")
	}
	for _, required := range []string{
		"Consume(ctx context.Context, sessionID, runID string)",
		"RequireResumeClaim(ctx context.Context, sessionID, runID string)",
		"Delete(ctx context.Context, sessionID, runID string)",
		"WHERE session_id = ? AND root_run_id = ?",
		"state != \"resuming\"",
		"rejectForeignPendingOwner",
	} {
		if !strings.Contains(string(interruptSource), required) {
			t.Errorf("interrupt store no longer enforces owner marker %q", required)
		}
	}

	terminalPlanPath := filepath.Join(root, "internal", "application", "sessions", "write_set.go")
	terminalPlanSource, err := os.ReadFile(terminalPlanPath)
	if err != nil {
		t.Fatalf("read terminal write-set: %v", err)
	}
	for _, required := range []string{
		"Runs             []transcript.Run",
		"rundomain.NewRunTree",
		"tree.Postorder()",
		"validateTerminalGoalRun",
	} {
		if !strings.Contains(string(terminalPlanSource), required) {
			t.Errorf("parked-tree terminal write-set no longer enforces %q", required)
		}
	}
	terminalUseCasePath := filepath.Join(root, "internal", "application", "sessions", "interrupt.go")
	terminalUseCaseSource, err := os.ReadFile(terminalUseCasePath)
	if err != nil {
		t.Fatalf("read parked-tree terminal use case: %v", err)
	}
	for _, required := range []string{"pending.Continuations", "terminalRuns", "plan.GoalRun"} {
		if !strings.Contains(string(terminalUseCaseSource), required) {
			t.Errorf("parked-tree terminal use case no longer owns %q", required)
		}
	}

	recoveryPath := filepath.Join(root, "internal", "application", "runs", "recovery_commit.go")
	recoverySource, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("read recovery commit validator: %v", err)
	}
	for _, required := range []string{"func (commit RecoveryCommit) Validate() error", "validateRecoveryGoalRuns", "validateRecoveryInterruptDeletions"} {
		if !strings.Contains(string(recoverySource), required) {
			t.Errorf("recovery write-set no longer validates %q", required)
		}
	}

	runSchemaPath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "db.go")
	runSchema, err := os.ReadFile(runSchemaPath)
	if err != nil {
		t.Fatalf("read SQLite schema: %v", err)
	}
	for _, required := range []string{"goal_lease_id", "root_run_id = '' OR goal_lease_id = ''"} {
		if !strings.Contains(string(runSchema), required) {
			t.Errorf("Run admission no longer owns goal lease marker %q", required)
		}
	}
}

// TestParkedContinuationFactsAreCrossCheckedAtEveryLifecycleBoundary prevents
// Pending from becoming a second mutable author for Run admission/accounting.
// Local Validate calls are necessary but insufficient: the frozen hand-off must
// also equal the Run row at creation, boot recovery, and online teardown.
func TestParkedContinuationFactsAreCrossCheckedAtEveryLifecycleBoundary(t *testing.T) {
	root := moduleRoot(t)
	checks := map[string][]string{
		filepath.Join("internal", "application", "runs", "pending.go"): {
			"p.Capabilities.Validate()",
			"p.Capabilities.ChildRuns",
			"slices.Contains(p.Capabilities.InterruptKinds, interrupt.Kind)",
		},
		filepath.Join("internal", "application", "runs", "commit.go"): {
			"commit.Run.Metrics.Equal(continuation.Metrics)",
			"commit.Run.Limits != continuation.Limits",
			"commit.Run.Capabilities.Equal(c.Pending.Capabilities)",
			"commit.Run.GoalLeaseID != c.Pending.GoalLeaseID",
		},
		filepath.Join("internal", "application", "runs", "waiting_cancellation_commit.go"): {
			"run.Metrics.Equal(continuation.Metrics)",
			"run.Limits != continuation.Limits",
			"c.RootRun.Capabilities.Equal(c.ExpectedPending.Capabilities)",
		},
		filepath.Join("internal", "application", "runs", "cancel_plan.go"): {
			"validatePendingRunTree(*pending, activeRuns)",
		},
		filepath.Join("internal", "application", "runs", "resume.go"): {
			"validatePendingRunTree(pending, parkedRuns)",
		},
		filepath.Join("internal", "application", "runs", "recovery_validation.go"): {
			"validateContinuationRunFacts(root.ID, active, continuation)",
			"pending.Capabilities.Equal(tree.root.Capabilities)",
		},
		filepath.Join("internal", "application", "runs", "continuation_validation.go"): {
			"run.Metrics.Equal(continuation.Metrics)",
			"run.Limits != continuation.Limits",
			"run.ModelSelection != continuation.ModelSelection",
		},
		filepath.Join("internal", "application", "sessions", "interrupt.go"): {
			"run.Metrics.Equal(continuation.Metrics)",
			"run.Limits != continuation.Limits",
			"pending.Capabilities.Equal(rootAdmission.Capabilities)",
		},
	}
	for relative, required := range checks {
		source, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, marker := range required {
			if !strings.Contains(string(source), marker) {
				t.Errorf("%s no longer cross-checks parked fact %q", relative, marker)
			}
		}
	}
}

func assertAgentexecHasNoCheckpointWrites(t *testing.T, root string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "internal", "adapter", "agentexec", "*.go"))
	if err != nil {
		t.Fatalf("list agentexec files: %v", err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			for _, forbidden := range []string{
				"SaveCheckpoint", "DeleteCheckpoints", "DeleteSessionCheckpoints", "DeleteUnownedCheckpoints",
			} {
				if selectorCallExists(function.Body, forbidden) {
					t.Errorf("agentexec function %s.%s performs application-owned %s", receiverName(function.Recv), function.Name.Name, forbidden)
				}
			}
		}
	}
}

func structFields(file *ast.File, name string) []string {
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range generic.Specs {
			named, ok := spec.(*ast.TypeSpec)
			if !ok || named.Name.Name != name {
				continue
			}
			structure, ok := named.Type.(*ast.StructType)
			if !ok {
				return nil
			}
			var fields []string
			for _, field := range structure.Fields.List {
				for _, fieldName := range field.Names {
					fields = append(fields, fieldName.Name)
				}
			}
			return fields
		}
	}
	return nil
}

// TestRuntimeDoesNotConfigureSingleProcessPreparedStepDurability freezes
// ADR-RT-039: Runtime can recover only complete quiescent trees and therefore
// must not imply tree durability by acknowledging one Process snapshot.
func TestRuntimeDoesNotConfigureSingleProcessPreparedStepDurability(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "EngineConfig" {
				return true
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				name, ok := field.Key.(*ast.Ident)
				if ok && name.Name == "PreparedStepAcknowledger" {
					relative, _ := filepath.Rel(moduleRoot(t), path)
					t.Errorf("Runtime configured single-Process prepared-step durability in %s", relative)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan Agent2 Engine configuration: %v", err)
	}
}

func methodBody(file *ast.File, receiver, name string) *ast.BlockStmt {
	if file == nil {
		return nil
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != name || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		receiverName := strings.TrimPrefix(exprString(function.Recv.List[0].Type), "*")
		if receiverName == receiver {
			return function.Body
		}
	}
	return nil
}

func selectorCallExists(node ast.Node, name string) bool {
	if node == nil {
		return false
	}
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func calledClosure(body *ast.BlockStmt, name string) (*ast.FuncLit, bool) {
	if body == nil {
		return nil, false
	}
	var closure *ast.FuncLit
	ast.Inspect(body, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return true
		}
		for _, argument := range call.Args {
			if function, ok := argument.(*ast.FuncLit); ok {
				closure = function
				return false
			}
		}
		return true
	})
	return closure, closure != nil
}
