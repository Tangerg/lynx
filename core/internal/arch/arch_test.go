// Package arch holds architecture-fitness tests for the core module.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestDocumentRemainsPureData(t *testing.T) {
	typ := reflect.TypeFor[document.Document]()
	wantFields := []string{"ID", "Text", "Media", "Metadata"}
	for index, name := range wantFields {
		if index >= typ.NumField() {
			t.Fatalf("document.Document missing field %d (%s)", index, name)
		}
		if got := typ.Field(index).Name; got != name {
			t.Fatalf("document.Document field %d = %v, want %s", index, got, name)
		}
	}
	if typ.NumField() != len(wantFields) {
		t.Fatalf("document.Document has %d fields, want %d", typ.NumField(), len(wantFields))
	}
	metadataField, _ := typ.FieldByName("Metadata")
	if metadataField.Type != reflect.TypeFor[metadata.Map]() {
		t.Fatalf("document.Metadata type = %v, want metadata.Map", metadataField.Type)
	}

	pointer := reflect.PointerTo(typ)
	for _, forbidden := range []string{"EnsureID", "Format", "FormatByMetadataMode", "FormatWith"} {
		if _, ok := pointer.MethodByName(forbidden); ok {
			t.Errorf("document.Document must not expose runtime method %s", forbidden)
		}
	}
}

func TestVectorStoreCapabilitiesRemainSmall(t *testing.T) {
	want := map[reflect.Type]string{
		reflect.TypeFor[vectorstore.Indexer]():       "Add",
		reflect.TypeFor[vectorstore.Searcher]():      "Search",
		reflect.TypeFor[vectorstore.IDDeleter]():     "DeleteIDs",
		reflect.TypeFor[vectorstore.FilterDeleter](): "DeleteWhere",
	}
	for typ, method := range want {
		if typ.NumMethod() != 1 || typ.Method(0).Name != method {
			t.Errorf("%v methods changed: want only %s", typ, method)
		}
	}

	root := filepath.Join(moduleRoot(t), "vectorstore")
	forbidden := map[string]bool{
		"Store": true, "StoreMetadata": true,
		"Creator": true, "CreateRequest": true,
		"Retriever": true, "RetrievalRequest": true,
		"Deleter": true, "DeleteRequest": true,
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				name := specification.(*ast.TypeSpec).Name.Name
				if forbidden[name] {
					t.Errorf("core/vectorstore must not reintroduce %s", name)
				}
			}
		}
	}
}

func TestEmbeddingSPIRemainsMinimal(t *testing.T) {
	want := map[reflect.Type]string{
		reflect.TypeFor[embedding.Model]():       "Call",
		reflect.TypeFor[embedding.Dimensioner](): "Dimensions",
	}
	for typ, method := range want {
		if typ.NumMethod() != 1 || typ.Method(0).Name != method {
			t.Errorf("%v methods changed: want only %s", typ, method)
		}
	}

	root := filepath.Join(moduleRoot(t), "embedding")
	forbiddenTypes := map[string]bool{
		"ModelMetadata": true,
		"ClientRequest": true, "ClientCaller": true,
		"Middleware": true, "MiddlewareChain": true, "Handler": true,
	}
	forbiddenFuncs := map[string]bool{
		"GetDimensions":    true,
		"NewClientRequest": true, "NewClientFromRequest": true,
		"NewMiddlewareChain": true,
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, specification := range typed.Specs {
					name := specification.(*ast.TypeSpec).Name.Name
					if forbiddenTypes[name] {
						t.Errorf("core/embedding must not reintroduce %s", name)
					}
				}
			case *ast.FuncDecl:
				if typed.Recv == nil && forbiddenFuncs[typed.Name.Name] {
					t.Errorf("core/embedding must not reintroduce %s", typed.Name.Name)
				}
			}
		}
	}
}

func TestOtherModalitySPIsRemainMinimal(t *testing.T) {
	want := map[reflect.Type]string{
		reflect.TypeFor[image.Model]():         "Call",
		reflect.TypeFor[transcription.Model](): "Call",
		reflect.TypeFor[moderation.Model]():    "Call",
		reflect.TypeFor[speech.Model]():        "Call",
		reflect.TypeFor[speech.Streamer]():     "Stream",
	}
	for typ, method := range want {
		if typ.NumMethod() != 1 || typ.Method(0).Name != method {
			t.Errorf("%v methods changed: want only %s", typ, method)
		}
	}

	for _, packageName := range []string{"image", "transcription", "speech", "moderation"} {
		assertMinimalModalityPackage(t, packageName)
	}
}

func assertMinimalModalityPackage(t *testing.T, packageName string) {
	t.Helper()
	root := filepath.Join(moduleRoot(t), packageName)
	forbiddenTypes := map[string]bool{
		"ModelMetadata": true, "Client": true,
		"ClientRequest": true, "ClientCaller": true, "ClientStreamer": true,
		"Middleware": true, "MiddlewareChain": true,
		"Handler": true, "HandlerFunc": true,
	}
	forbiddenFuncs := map[string]bool{
		"NewClient": true, "NewClientRequest": true,
		"NewClientFromRequest": true, "NewMiddlewareChain": true,
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "github.com/Tangerg/lynx/core/model" {
				t.Errorf("core/%s must not depend on the generic model framework: %s", packageName, path)
			}
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, specification := range typed.Specs {
					name := specification.(*ast.TypeSpec).Name.Name
					if forbiddenTypes[name] {
						t.Errorf("core/%s must not reintroduce %s", packageName, name)
					}
				}
			case *ast.FuncDecl:
				if typed.Recv == nil && forbiddenFuncs[typed.Name.Name] {
					t.Errorf("core/%s must not reintroduce %s", packageName, typed.Name.Name)
				}
			}
		}
	}
}

func TestFilterPublicFacadeExcludesCompilerInternals(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "vectorstore", "filter")
	forbiddenTypes := map[string]bool{
		"Token": true, "Lexer": true, "Parser": true,
		"Analyzer": true, "Optimizer": true, "Visitor": true,
	}
	forbiddenFuncs := map[string]bool{
		"Analyze": true, "Optimize": true, "ParseAndAnalyze": true,
		"NewLexer": true, "NewParser": true, "NewAnalyzer": true, "NewOptimizer": true,
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, specification := range typed.Specs {
					name := specification.(*ast.TypeSpec).Name.Name
					if forbiddenTypes[name] {
						t.Errorf("core/vectorstore/filter must not expose compiler type %s", name)
					}
				}
			case *ast.FuncDecl:
				if typed.Recv == nil && forbiddenFuncs[typed.Name.Name] {
					t.Errorf("core/vectorstore/filter must not expose compiler helper %s", typed.Name.Name)
				}
			}
		}
	}

	for _, typ := range []reflect.Type{
		reflect.TypeFor[filter.Ident](),
		reflect.TypeFor[filter.Literal](),
		reflect.TypeFor[filter.ListLiteral](),
		reflect.TypeFor[filter.UnaryExpr](),
		reflect.TypeFor[filter.BinaryExpr](),
		reflect.TypeFor[filter.IndexExpr](),
	} {
		for i := range typ.NumField() {
			if containsInternalType(typ.Field(i).Type) {
				t.Errorf("public filter node %v field %s exposes internal type %v", typ, typ.Field(i).Name, typ.Field(i).Type)
			}
		}
	}
}

func containsInternalType(typ reflect.Type) bool {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	return strings.Contains(typ.PkgPath(), "/internal/")
}

func TestTargetChatSPIExcludesDefaultsAndIdentity(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "chat")
	fset := token.NewFileSet()
	allowed := map[string]map[string]bool{
		"Model":    {"Call": true},
		"Streamer": {"Stream": true},
	}
	found := make(map[string]bool, len(allowed))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read target chat package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/Tangerg/lynx/core/model" || strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/model/") {
				t.Errorf("target core/chat must not depend on legacy generic model layer %q: %s", importPath, path)
			}
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if typeSpec.Name.Name == "ModelMetadata" {
					t.Errorf("target core/chat must not expose provider identity type ModelMetadata: %s", path)
				}
				methods, tracked := allowed[typeSpec.Name.Name]
				if !tracked {
					continue
				}
				found[typeSpec.Name.Name] = true
				if typeSpec.TypeParams != nil && len(typeSpec.TypeParams.List) != 0 {
					t.Errorf("core/chat.%s must not use type parameters", typeSpec.Name.Name)
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					t.Errorf("core/chat.%s must remain an interface", typeSpec.Name.Name)
					continue
				}
				for _, field := range iface.Methods.List {
					if len(field.Names) == 0 {
						t.Errorf("core/chat.%s must not embed another interface", typeSpec.Name.Name)
						continue
					}
					for _, name := range field.Names {
						if !methods[name.Name] {
							t.Errorf("core/chat.%s must not require %s", typeSpec.Name.Name, name.Name)
						}
					}
				}
			}
		}
	}
	for name := range allowed {
		if !found[name] {
			t.Errorf("target core/chat.%s declaration not found", name)
		}
	}
}

func TestCoreModelExcludesAgentControlFlow(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "model")
	fset := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read core/model: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				switch typeSpec.Name.Name {
				case "Halt", "ControlFlowError":
					t.Errorf("core/model must not own agent control-flow type %s: %s", typeSpec.Name.Name, path)
				}
			}
		}
	}
}

func TestCoreDoesNotImportUpperLynxModules(t *testing.T) {
	const lynxPrefix = "github.com/Tangerg/lynx/"
	fset := token.NewFileSet()

	violations := 0
	for _, path := range productionGoFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			rest, ok := strings.CutPrefix(ip, lynxPrefix)
			if !ok {
				continue
			}
			if strings.HasPrefix(rest, "core/") || rest == "core" || strings.HasPrefix(rest, "pkg/") || rest == "pkg" {
				continue
			}
			violations++
			rel, _ := filepath.Rel(moduleRoot(t), path)
			t.Errorf("core must not import upper lynx module %q: %s", ip, rel)
		}
	}
	if violations == 0 {
		t.Log("core import boundary holds: only core/pkg lynx imports found")
	}
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)
	var files []string
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk core: %v", walkErr)
	}
	return files
}

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
