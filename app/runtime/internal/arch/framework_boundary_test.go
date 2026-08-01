package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestProcessCheckpointsDoNotDeriveProductSessions prevents the dual-identity
// leak removed with schema epoch 46. Executor process identity belongs to the
// opaque checkpoint aggregate; it must never be materialized as a product
// conversation merely because a process is a child.
func TestProcessCheckpointsDoNotDeriveProductSessions(t *testing.T) {
	root := moduleRoot(t)
	processPath := filepath.Join(root, "internal", "infra", "storage", "sqlite", "process.go")
	processFile, err := parser.ParseFile(token.NewFileSet(), processPath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse process store: %v", err)
	}
	for _, imported := range processFile.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if path == "github.com/Tangerg/lynx/app/runtime/internal/domain/session" {
			t.Errorf("process checkpoint store imports product Session domain: %s", processPath)
		}
	}

	sessionPath := filepath.Join(root, "internal", "domain", "session", "session.go")
	sessionFile, err := parser.ParseFile(token.NewFileSet(), sessionPath, nil, 0)
	if err != nil {
		t.Fatalf("parse Session domain: %v", err)
	}
	forbidden := map[string]struct{}{
		"Subtask": {}, "KindSubtask": {}, "NewSubtask": {}, "ErrInvalidSubtask": {},
	}
	for _, declaration := range sessionFile.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if _, leaked := forbidden[value.Name.Name]; leaked {
				t.Errorf("product Session domain restores executor-derived declaration %s", value.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				switch named := spec.(type) {
				case *ast.TypeSpec:
					if _, leaked := forbidden[named.Name.Name]; leaked {
						t.Errorf("product Session domain restores executor-derived type %s", named.Name.Name)
					}
				case *ast.ValueSpec:
					for _, name := range named.Names {
						if _, leaked := forbidden[name.Name]; leaked {
							t.Errorf("product Session domain restores executor-derived value %s", name.Name)
						}
					}
				}
			}
		}
	}
}

// TestWaitingCheckpointPersistenceBelongsToTreeBarrier locks the App policy
// boundary: joining an Agent segment is pure execution observation, while the
// application tree barrier owns checkpoint + Pending + Run atomicity.
func TestWaitingCheckpointPersistenceBelongsToTreeBarrier(t *testing.T) {
	root := moduleRoot(t)
	assertAgentexecCheckpointIOOwners(t, root)

	turnPath := filepath.Join(root, "internal", "adapter", "agentexec", "turnprocess.go")
	turnFile, err := parser.ParseFile(token.NewFileSet(), turnPath, nil, 0)
	if err != nil {
		t.Fatalf("parse turn process: %v", err)
	}
	await := methodBody(turnFile, "turnProcess", "Await")
	if await == nil {
		t.Fatal("turnProcess.Await not found")
	}
	for _, forbidden := range []string{"PersistCheckpoint", "SaveTree"} {
		if selectorCallExists(await, forbidden) {
			t.Errorf("turnProcess.Await performs application-owned persistence through %s", forbidden)
		}
	}
	discard := methodBody(turnFile, "turnProcess", "Discard")
	if discard == nil {
		t.Fatal("turnProcess.Discard not found")
	}
	for _, forbidden := range []string{"DeleteTrees", "DeleteSessionTrees"} {
		if selectorCallExists(discard, forbidden) {
			t.Errorf("turnProcess.Discard performs application-owned persistence through %s", forbidden)
		}
	}

	commitPath := filepath.Join(root, "internal", "adapter", "runsegment", "effects_commit.go")
	commitFile, err := parser.ParseFile(token.NewFileSet(), commitPath, nil, 0)
	if err != nil {
		t.Fatalf("parse runsegment effects: %v", err)
	}
	commit := methodBody(commitFile, "Effects", "CommitTreeBarrier")
	if commit == nil {
		t.Fatal("Effects.CommitTreeBarrier not found")
	}
	transaction, ok := calledClosure(commit, "runInTx")
	if !ok {
		t.Fatal("CommitTreeBarrier does not execute one application transaction")
	}
	for _, required := range []string{"PersistCheckpoint", "putInterrupt", "applyCommit"} {
		if !selectorCallExists(transaction.Body, required) {
			t.Errorf("CommitTreeBarrier transaction does not own %s", required)
		}
	}
	terminalCommit := methodBody(commitFile, "Effects", "CommitEvent")
	if terminalCommit == nil {
		t.Fatal("Effects.CommitEvent not found")
	}
	terminalTransaction, ok := calledClosure(terminalCommit, "runInTx")
	if !ok || !selectorCallExists(terminalTransaction.Body, "DeleteTrees") {
		t.Error("CommitEvent terminal transaction does not own process checkpoint deletion")
	}
}

// assertAgentexecCheckpointIOOwners scans the complete production adapter
// package, not just TurnProcess's visible methods. That prevents a persistence
// side effect from returning through an innocently named helper.
func assertAgentexecCheckpointIOOwners(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "internal", "adapter", "agentexec")
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
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
			for _, forbidden := range []string{"DeleteTrees", "DeleteSessionTrees", "DeleteUnownedTrees"} {
				if selectorCallExists(function.Body, forbidden) {
					t.Errorf("agentexec production function %s.%s performs application-owned deletion through %s", receiverName(function.Recv), function.Name.Name, forbidden)
				}
			}
			if !selectorCallExists(function.Body, "SaveTree") {
				continue
			}
			if filepath.Base(path) != "turnprocess.go" ||
				receiverName(function.Recv) != "capturedProcessTree" ||
				function.Name.Name != "PersistCheckpoint" {
				t.Errorf("agentexec checkpoint SaveTree is owned by %s.%s in %s; want capturedProcessTree.PersistCheckpoint", receiverName(function.Recv), function.Name.Name, path)
			}
		}
	}
}

func methodBody(file *ast.File, receiver, name string) *ast.BlockStmt {
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
