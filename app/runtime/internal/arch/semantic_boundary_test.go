package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestGitProcessEnvironmentHasOneOwner keeps repository routing and process
// behavior below one infrastructure boundary. Direct Git subprocesses can
// silently inherit a parent process's GIT_DIR, index, object database, config,
// or pathspec controls even when the caller supplies -C.
func TestGitProcessEnvironmentHasOneOwner(t *testing.T) {
	root := moduleRoot(t)
	owner := filepath.Join(root, "internal", "infra", "gitprocess")
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || filepath.Dir(path) == owner {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		execAliases := map[string]struct{}{}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) != "os/exec" {
				continue
			}
			name := "exec"
			if imported.Name != nil {
				name = imported.Name.Name
			}
			execAliases[name] = struct{}{}
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
			alias, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, isExec := execAliases[alias.Name]; !isExec {
				return true
			}
			argument := 0
			if selector.Sel.Name == "CommandContext" {
				argument = 1
			} else if selector.Sel.Name != "Command" {
				return true
			}
			if len(call.Args) > argument && gitLiteral(call.Args[argument]) {
				t.Errorf("%s launches Git outside internal/infra/gitprocess", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan Git process ownership: %v", err)
	}
}

func gitLiteral(expression ast.Expr) bool {
	literal, ok := expression.(*ast.BasicLit)
	return ok && literal.Kind == token.STRING && literal.Value == `"git"`
}

func TestMediaContentEncodingStaysAtOuterBoundaries(t *testing.T) {
	root := moduleRoot(t)
	applicationTokenFraming := filepath.Join(root, "internal", "application", "opaquetoken")
	modelPath := filepath.Join(root, "internal", "domain", "transcript", "model.go")
	model, err := parser.ParseFile(token.NewFileSet(), modelPath, nil, 0)
	if err != nil {
		t.Fatalf("parse transcript model: %v", err)
	}
	wantFields := []string{"Kind", "Text", "MediaType", "Bytes"}
	if fields := structFields(model, "ContentBlock"); !slices.Equal(fields, wantFields) {
		t.Fatalf("ContentBlock fields = %v, want semantic media value %v", fields, wantFields)
	}

	for _, ring := range []string{"domain", "application"} {
		err := filepath.WalkDir(filepath.Join(root, "internal", ring), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imported := range file.Imports {
				name := strings.Trim(imported.Path.Value, `"`)
				// URL-safe continuation framing is an Application contract, not
				// media content encoding owned by a transport adapter.
				if name == "encoding/base64" && filepath.Dir(path) == applicationTokenFraming {
					continue
				}
				if name == "encoding/base64" || name == "mime" {
					t.Errorf("%s imports media content codec %q", path, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s content codecs: %v", ring, err)
		}
	}
}

func TestDomainValuesDoNotOwnJSONPersistenceCodecs(t *testing.T) {
	root := moduleRoot(t)
	domain := filepath.Join(root, "internal", "domain")
	allowed := map[string]map[string]struct{}{
		filepath.Join("tool", "value.go"): {
			"Arguments.MarshalJSON": {}, "Arguments.UnmarshalJSON": {},
			"Result.MarshalJSON": {}, "Result.UnmarshalJSON": {},
		},
	}
	err := filepath.WalkDir(domain, func(path string, entry fs.DirEntry, walkErr error) error {
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
		relative, relativeErr := filepath.Rel(domain, path)
		if relativeErr != nil {
			return relativeErr
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || (function.Name.Name != "MarshalJSON" && function.Name.Name != "UnmarshalJSON") {
				continue
			}
			receiver := receiverName(function.Recv)
			key := receiver + "." + function.Name.Name
			if _, ok := allowed[relative][key]; !ok {
				t.Errorf("%s gives domain value %s a JSON boundary codec", relative, key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan domain JSON codecs: %v", err)
	}
}

func TestSQLiteOwnsTranscriptAndInterruptPayloadShapes(t *testing.T) {
	root := moduleRoot(t)
	transcriptStore := filepath.Join(root, "internal", "infra", "sqlite", "transcript.go")
	transcriptSource, err := os.ReadFile(transcriptStore)
	if err != nil {
		t.Fatalf("read transcript store: %v", err)
	}
	if strings.Contains(string(transcriptSource), "encoding/json") || strings.Contains(string(transcriptSource), "json.Marshal") || strings.Contains(string(transcriptSource), "json.Unmarshal") {
		t.Error("transcript store serializes the domain aggregate instead of using its explicit adapter codec")
	}

	interruptStore := filepath.Join(root, "internal", "infra", "sqlite", "interrupt.go")
	interruptSource, err := os.ReadFile(interruptStore)
	if err != nil {
		t.Fatalf("read interrupt store: %v", err)
	}
	for _, leaked := range []string{
		"json.Marshal(p.Interrupts)",
		"var out []transcript.Interrupt",
		"Problem transcript.Problem",
	} {
		if strings.Contains(string(interruptSource), leaked) {
			t.Errorf("interrupt store restores implicit domain JSON shape %q", leaked)
		}
	}
}

func TestRunCapabilitiesStaySemanticInsideTheProtocolBoundary(t *testing.T) {
	root := moduleRoot(t)
	capabilitiesPath := filepath.Join(root, "internal", "domain", "run", "capabilities.go")
	capabilitiesFile, err := parser.ParseFile(token.NewFileSet(), capabilitiesPath, nil, 0)
	if err != nil {
		t.Fatalf("parse Run capabilities: %v", err)
	}
	wantFields := []string{"ChildRuns", "InterruptKinds"}
	if fields := structFields(capabilitiesFile, "Capabilities"); !slices.Equal(fields, wantFields) {
		t.Fatalf("Capabilities fields = %v, want semantic behavior %v", fields, wantFields)
	}

	for _, relative := range []string{
		filepath.Join("internal", "domain"),
		filepath.Join("internal", "application"),
		filepath.Join("internal", "adapter", "runsegment"),
		filepath.Join("internal", "infra", "sqlite"),
	} {
		walkDirErr := filepath.WalkDir(filepath.Join(root, relative), func(path string, entry fs.DirEntry, walkErr error) error {
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
			for _, leaked := range []string{"RunProtocolProfile", "ProtocolProfile", "protocol_profile"} {
				if strings.Contains(string(source), leaked) {
					t.Errorf("%s leaks protocol vocabulary %q inward", path, leaked)
				}
			}
			return nil
		})
		if walkDirErr != nil {
			t.Fatalf("scan %s Run capability vocabulary: %v", relative, walkDirErr)
		}
	}

	protocolSource, err := os.ReadFile(filepath.Join(root, "protocol", "runs.go"))
	if err != nil {
		t.Fatalf("read Run protocol: %v", err)
	}
	if !strings.Contains(string(protocolSource), "type RunProtocolProfile struct") {
		t.Error("Delivery no longer owns the versioned RunProtocolProfile wire shape")
	}
	codecSource, err := os.ReadFile(filepath.Join(root, "internal", "infra", "sqlite", "run_codec.go"))
	if err != nil {
		t.Fatalf("read Run capability codec: %v", err)
	}
	if strings.Contains(string(codecSource), `json:"interruptTypes`) || !strings.Contains(string(codecSource), `json:"interruptKinds`) {
		t.Error("SQLite Run capability codec uses protocol interruptTypes instead of semantic interruptKinds")
	}
}

func TestExecutorAndGoalPersistentShapesStayCanonical(t *testing.T) {
	root := moduleRoot(t)
	assertExecutorIdentityShapes(t, root)
	assertRetiredDurableVocabularyAbsent(t, root)
}

func assertExecutorIdentityShapes(t *testing.T, root string) {
	t.Helper()
	executorRefPath := filepath.Join(root, "internal", "application", "runs", "executor_ref.go")
	executorRefFile, err := parser.ParseFile(token.NewFileSet(), executorRefPath, nil, 0)
	if err != nil {
		t.Fatalf("parse executor reference: %v", err)
	}
	if fields := structFields(executorRefFile, "ExecutorRef"); !slices.Equal(fields, []string{"SessionID", "ExecutorID"}) {
		t.Fatalf("ExecutorRef fields = %v, want durable executor identity", fields)
	}

	scopePath := filepath.Join(root, "internal", "application", "runs", "executor_checkpoint.go")
	scopeFile, err := parser.ParseFile(token.NewFileSet(), scopePath, nil, 0)
	if err != nil {
		t.Fatalf("parse execution scope: %v", err)
	}
	wantScope := []string{"SessionID", "CWD", "WorkspaceCWD", "Isolated", "GoalIncarnationID"}
	if fields := structFields(scopeFile, "ExecutionScope"); !slices.Equal(fields, wantScope) {
		t.Fatalf("ExecutionScope fields = %v, want %v", fields, wantScope)
	}
}

func assertRetiredDurableVocabularyAbsent(t *testing.T, root string) {
	t.Helper()
	sqliteDir := filepath.Join(root, "internal", "infra", "sqlite")
	err := filepath.WalkDir(sqliteDir, func(path string, entry fs.DirEntry, walkErr error) error {
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
		for _, retired := range []string{"turn_id", "goal_turns", "max_turns", `json:"turns"`, "turnBudgetReached"} {
			if strings.Contains(string(source), retired) {
				t.Errorf("%s retains retired durable vocabulary %q", path, retired)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan SQLite durable vocabulary: %v", err)
	}
}

func TestReplayRetentionDoesNotDependOnAnOuterEncoding(t *testing.T) {
	root := moduleRoot(t)
	runs := filepath.Join(root, "internal", "application", "runs")
	for _, name := range []string{"journal.go", "journal_retention.go"} {
		path := filepath.Join(runs, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "encoding/json" {
				t.Errorf("%s derives retained memory from JSON encoding", name)
			}
		}
	}

	eventPath := filepath.Join(runs, "run_event.go")
	eventFile, err := parser.ParseFile(token.NewFileSet(), eventPath, nil, 0)
	if err != nil {
		t.Fatalf("parse RunEvent: %v", err)
	}
	methods := interfaceMethods(eventFile, "RunEvent")
	if !slices.Contains(methods, "retainedBytes") {
		t.Fatal("RunEvent no longer makes retention accounting mandatory for every event variant")
	}
}

func interfaceMethods(file *ast.File, name string) []string {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec := specification.(*ast.TypeSpec)
			if typeSpec.Name.Name != name {
				continue
			}
			contract, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				return nil
			}
			var methods []string
			for _, field := range contract.Methods.List {
				for _, method := range field.Names {
					methods = append(methods, method.Name)
				}
			}
			return methods
		}
	}
	return nil
}
