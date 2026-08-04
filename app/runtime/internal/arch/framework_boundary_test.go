package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestExecutorCheckpointRemainsOpaqueOutsideExecutionAdapter prevents App from
// reconstructing framework process topology. App may own only the aggregate
// root identity, opaque payload, and host metadata required for lifecycle
// policy.
func TestExecutorCheckpointRemainsOpaqueOutsideExecutionAdapter(t *testing.T) {
	root := moduleRoot(t)
	checkpointPath := filepath.Join(root, "internal", "domain", "execution", "executor_checkpoint.go")
	checkpointFile, err := parser.ParseFile(token.NewFileSet(), checkpointPath, nil, 0)
	if err != nil {
		t.Fatalf("parse executor checkpoint: %v", err)
	}
	checkpoint := structFields(checkpointFile, "ExecutorCheckpoint")
	want := []string{"RootProcessID", "Payload", "BuildID", "Scope", "ModelSelection", "Limits", "Usage"}
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
	domainPath := filepath.Join(root, "internal", "domain", "execution", "admission.go")
	domainFile, err := parser.ParseFile(token.NewFileSet(), domainPath, nil, 0)
	if err != nil {
		t.Fatalf("parse execution admission: %v", err)
	}
	wantLimits := []string{"MaxTotalTokens", "MaxSteps", "MaxBudgetUSD"}
	if fields := structFields(domainFile, "RunLimits"); strings.Join(fields, ",") != strings.Join(wantLimits, ",") {
		t.Fatalf("RunLimits fields = %v, want the single policy carrier %v", fields, wantLimits)
	}

	accountingRoot := filepath.Join(root, "internal", "domain", "execution", "accounting")
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
		{path: filepath.Join("internal", "application", "runs", "ports.go"), name: "StartTurn"},
		{path: filepath.Join("internal", "adapter", "agentexec", "turnrun.go"), name: "TurnRequest"},
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
// turn scope from being nested under agentexec again.
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

// TestPendingStoresOnlyOpaqueExecutorBindings prevents the App continuation
// from persisting a second copy of Framework parent/spawn topology. Run lineage
// is the durable product tree; live executor topology is validated only while
// routing events through the adapter boundary.
func TestPendingStoresOnlyOpaqueExecutorBindings(t *testing.T) {
	root := moduleRoot(t)
	interruptPath := filepath.Join(root, "internal", "domain", "execution", "interrupts", "interrupts.go")
	interruptFile, err := parser.ParseFile(token.NewFileSet(), interruptPath, nil, 0)
	if err != nil {
		t.Fatalf("parse interrupt domain: %v", err)
	}
	want := []string{
		"RunID", "ProcessID", "Lineage", "ModelSelection", "DrainedTools",
		"CommittedTools", "RunCreatedAt", "Metrics", "Limits",
	}
	if fields := structFields(interruptFile, "Continuation"); strings.Join(fields, ",") != strings.Join(want, ",") {
		t.Fatalf("Continuation fields = %v, want App facts plus one opaque executor binding %v", fields, want)
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
// contract aligned with the product model. Framework process identities remain
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
		t.Fatalf("SubagentInput fields = %v, want App lifecycle identity %v", fields, want)
	}

	shellPath := filepath.Join(root, "internal", "adapter", "hooks", "shell.go")
	shellSource, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatalf("read hook shell adapter: %v", err)
	}
	for _, required := range []string{"json:\"runId\"", "json:\"parentRunId,omitempty\""} {
		if !strings.Contains(string(shellSource), required) {
			t.Errorf("hook JSON no longer exposes App identity marker %q", required)
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
// Shared tool vocabulary belongs to domain/tool; the HITL capability belongs to
// the runs consumer contract.
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

// TestDomainDoesNotNameOuterRings prevents documentation from rebuilding a
// reverse dependency in the reader's mental model after imports have been
// cleaned up. Domain source describes semantics and ports, never concrete
// application or adapter package locations.
func TestDomainDoesNotNameOuterRings(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal", "domain")
	forbidden := []string{
		"internal/adapter/",
		"internal/application/",
		"internal/delivery/",
		"internal/infra/",
		"internal/bootstrap/",
		"adapter/",
		"application/",
		"delivery/",
		"infra/",
		"bootstrap/",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, outer := range forbidden {
			if strings.Contains(string(source), outer) {
				t.Errorf("domain source %s names outer ring %q", path, outer)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk domain source: %v", err)
	}
}

// TestExecutorCheckpointBindingIsValidatedAtEveryBoundary locks the
// cross-aggregate invariant that a valid checkpoint and a valid Pending are
// insufficient unless they describe the same root, Session, model selection, goal
// lease, and restore workspace.
func TestExecutorCheckpointBindingIsValidatedAtEveryBoundary(t *testing.T) {
	root := moduleRoot(t)
	checks := map[string][]string{
		filepath.Join("internal", "application", "runs", "engine_event.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "application", "runs", "waiting_cancellation.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "adapter", "runsegment", "effects_commit.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
			"DeleteCheckpoints(ctx, commit.SessionID",
		},
		filepath.Join("internal", "adapter", "runsegment", "effects_waiting_cancellation.go"): {
			"ValidateOwnership", "Scope.GoalLeaseID", "Checkpoint.ModelSelection",
		},
		filepath.Join("internal", "application", "runs", "recovery_validation.go"): {
			"ExecutorCheckpointExpectation", "GoalLeaseID", "sess.Cwd", "sess.Isolated",
		},
		filepath.Join("internal", "adapter", "agentexec", "checkpoint_restore.go"): {
			"ValidateFor",
		},
		filepath.Join("internal", "adapter", "agentexec", "turnrun.go"): {
			"ValidateFor", "GoalLeaseID", "request.Cwd", "request.Isolated",
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
// becoming ambient mutation authority. A Pending opens once, every destructive
// operation names its Session owner, and recovery validates its complete
// write-set before persistence begins.
func TestPendingAndRecoveryMutationsCarryTheirOwners(t *testing.T) {
	root := moduleRoot(t)
	interruptPath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "interrupt.go")
	interruptSource, err := os.ReadFile(interruptPath)
	if err != nil {
		t.Fatalf("read interrupt store: %v", err)
	}
	for _, forbidden := range []string{"ON CONFLICT(root_run_id)"} {
		if strings.Contains(string(interruptSource), forbidden) {
			t.Errorf("interrupt store restores overwrite seam %q", forbidden)
		}
	}
	for _, required := range []string{
		"Open(ctx context.Context, p interrupts.Pending)",
		"Consume(ctx context.Context, sessionID, runID string)",
		"Delete(ctx context.Context, sessionID, runID string)",
		"WHERE session_id = ? AND root_run_id = ?",
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
		"execution.NewRunTree",
		"tree.Postorder()",
		"validateTerminalGoalTurn",
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
	for _, required := range []string{"pending.Continuations", "terminalRuns", "plan.GoalTurn"} {
		if !strings.Contains(string(terminalUseCaseSource), required) {
			t.Errorf("parked-tree terminal use case no longer owns %q", required)
		}
	}

	recoveryPath := filepath.Join(root, "internal", "application", "runs", "recovery_commit.go")
	recoverySource, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("read recovery commit validator: %v", err)
	}
	for _, required := range []string{"func (commit RecoveryCommit) Validate() error", "validateRecoveryGoalTurns", "validatePendingDeletions"} {
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
		filepath.Join("internal", "domain", "execution", "interrupts", "interrupts.go"): {
			"p.Capabilities.Validate()",
			"p.Capabilities.ChildRuns",
			"slices.Contains(p.Capabilities.InterruptKinds, interrupt.Kind)",
		},
		filepath.Join("internal", "adapter", "runsegment", "effects_commit.go"): {
			"commit.Run.Metrics.Equal(continuation.Metrics)",
			"commit.Run.Limits != continuation.Limits",
			"commit.Run.Capabilities.Equal(barrier.Pending.Capabilities)",
			"commit.Run.GoalLeaseID != barrier.Pending.GoalLeaseID",
		},
		filepath.Join("internal", "adapter", "runsegment", "effects_waiting_cancellation.go"): {
			"run.Metrics.Equal(continuation.Metrics)",
			"run.Limits != continuation.Limits",
			"commit.RootRun.Capabilities.Equal(commit.ExpectedPending.Capabilities)",
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
