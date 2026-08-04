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

func TestContentEncodingStaysAtOuterBoundaries(t *testing.T) {
	root := moduleRoot(t)
	modelPath := filepath.Join(root, "internal", "domain", "execution", "transcript", "model.go")
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
				if name == "encoding/base64" || name == "mime" {
					t.Errorf("%s imports transport content codec %q", path, name)
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
	transcriptStore := filepath.Join(root, "internal", "infra", "storage", "sqlite", "transcript.go")
	transcriptSource, err := os.ReadFile(transcriptStore)
	if err != nil {
		t.Fatalf("read transcript store: %v", err)
	}
	if strings.Contains(string(transcriptSource), "encoding/json") || strings.Contains(string(transcriptSource), "json.Marshal") || strings.Contains(string(transcriptSource), "json.Unmarshal") {
		t.Error("transcript store serializes the domain aggregate instead of using its explicit adapter codec")
	}

	interruptStore := filepath.Join(root, "internal", "infra", "storage", "sqlite", "interrupt.go")
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
