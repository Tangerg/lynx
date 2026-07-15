package arch

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

var updateWireFixtures = flag.Bool(
	"update-wire-fixtures",
	false,
	"replace the reviewed Core wire compatibility fixture",
)

// wireFixtureCoverage maps every exported JSON struct to the representative
// root that exercises it in wire_contracts.golden.json. Adding a JSON DTO is a
// compatibility decision and must update both this inventory and the fixture.
var wireFixtureCoverage = map[string]string{
	"chat.Choice":                    "chat_response",
	"chat.Message":                   "chat_request",
	"chat.Options":                   "chat_request",
	"chat.Part":                      "chat_request",
	"chat.Request":                   "chat_request",
	"chat.Response":                  "chat_response",
	"chat.ToolCall":                  "chat_request",
	"chat.ToolDefinition":            "chat_request",
	"chat.ToolResult":                "chat_request",
	"chat.Usage":                     "chat_response",
	"document.Document":              "document",
	"embedding.Options":              "embedding_request",
	"embedding.Request":              "embedding_request",
	"embedding.Response":             "embedding_response",
	"embedding.ResponseMetadata":     "embedding_response",
	"embedding.Result":               "embedding_response",
	"embedding.ResultMetadata":       "embedding_response",
	"embedding.Usage":                "embedding_response",
	"image.Options":                  "image_request",
	"image.Request":                  "image_request",
	"image.Response":                 "image_response",
	"image.ResponseMetadata":         "image_response",
	"image.Result":                   "image_response",
	"image.ResultMetadata":           "image_response",
	"media.Media":                    "media",
	"media.Source":                   "media",
	"moderation.Options":             "moderation_request",
	"moderation.Request":             "moderation_request",
	"moderation.Response":            "moderation_response",
	"moderation.ResponseMetadata":    "moderation_response",
	"moderation.Result":              "moderation_response",
	"moderation.ResultMetadata":      "moderation_response",
	"moderation.Verdict":             "moderation_response",
	"speech.Options":                 "speech_request",
	"speech.Request":                 "speech_request",
	"speech.Response":                "speech_response",
	"speech.ResponseMetadata":        "speech_response",
	"speech.Result":                  "speech_response",
	"speech.ResultMetadata":          "speech_response",
	"transcription.Options":          "transcription_request",
	"transcription.Request":          "transcription_request",
	"transcription.Response":         "transcription_response",
	"transcription.ResponseMetadata": "transcription_response",
	"transcription.Result":           "transcription_response",
	"transcription.ResultMetadata":   "transcription_response",
	"vectorstore.Match":              "vectorstore_match",
	"vectorstore.SearchRequest":      "vectorstore_search_request",
}

func TestWireTypeCoverage(t *testing.T) {
	t.Parallel()

	got := discoverExportedJSONStructs(t)
	want := make([]string, 0, len(wireFixtureCoverage))
	for name := range wireFixtureCoverage {
		want = append(want, name)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("exported JSON struct inventory changed\nwant: %v\n got: %v\nupdate wireFixtureCoverage and wire_contracts.golden.json", want, got)
	}
}

func TestWireContractsMatchGolden(t *testing.T) {
	got, err := json.MarshalIndent(representativeWireContracts(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	filename := filepath.Join(moduleRoot(t), "internal", "arch", "testdata", "wire_contracts.golden.json")
	if *updateWireFixtures {
		if err := os.WriteFile(filename, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Core wire contract changed\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func discoverExportedJSONStructs(t *testing.T) []string {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()
	var names []string
	for _, filename := range productionGoFiles(t) {
		packagePath, err := filepath.Rel(root, filepath.Dir(filename))
		if err != nil {
			t.Fatal(err)
		}
		packagePath = filepath.ToSlash(packagePath)
		if _, public := targetPublicPackages[packagePath]; !public {
			continue
		}

		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !ast.IsExported(typeSpec.Name.Name) || !hasJSONTag(structure) {
					continue
				}
				names = append(names, file.Name.Name+"."+typeSpec.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return names
}

func hasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err == nil && reflect.StructTag(tag).Get("json") != "" {
			return true
		}
	}
	return false
}
