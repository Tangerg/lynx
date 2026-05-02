package chat_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestListParser_Parse(t *testing.T) {
	p := chat.NewListParser()

	got, err := p.Parse(" apple , banana, cherry ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"apple", "banana", "cherry"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListParser_Instructions_Nonempty(t *testing.T) {
	if instr := chat.NewListParser().Instructions(); instr == "" {
		t.Fatal("Instructions must be a non-empty prompt fragment")
	}
}

func TestMapParser_Parse_BareJSON(t *testing.T) {
	p := chat.NewMapParser()

	got, err := p.Parse(`{"k":"v","n":42}`)
	if err != nil {
		t.Fatal(err)
	}
	if got["k"] != "v" {
		t.Fatalf("k = %v, want v", got["k"])
	}
	if got["n"].(float64) != 42 {
		t.Fatalf("n = %v, want 42", got["n"])
	}
}

func TestMapParser_Parse_StripsCodeFence(t *testing.T) {
	p := chat.NewMapParser()

	got, err := p.Parse("```json\n{\"k\":\"v\"}\n```")
	if err != nil {
		t.Fatalf("must strip ```json fences, got err: %v", err)
	}
	if got["k"] != "v" {
		t.Fatalf("k = %v, want v", got["k"])
	}
}

func TestMapParser_Parse_InvalidJSON(t *testing.T) {
	p := chat.NewMapParser()

	_, err := p.Parse("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "chat.MapParser.Parse") {
		t.Fatalf("error must include parser context, got: %q", err.Error())
	}
}

type recipe struct {
	Title string   `json:"title"`
	Steps []string `json:"steps"`
}

func TestJSONParser_Parse_Typed(t *testing.T) {
	p := chat.NewJSONParser[recipe]()

	got, err := p.Parse(`{"title":"pasta","steps":["boil","drain"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "pasta" {
		t.Fatalf("Title = %q, want pasta", got.Title)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("Steps len = %d, want 2", len(got.Steps))
	}
}

func TestJSONParser_Instructions_IncludesSchema(t *testing.T) {
	instr := chat.NewJSONParser[recipe]().Instructions()
	if !strings.Contains(instr, "JSON SCHEMA") {
		t.Fatal("Instructions should expose [JSON SCHEMA] marker")
	}
	if !strings.Contains(instr, "title") {
		t.Fatalf("schema should mention 'title' field; got: %s", instr)
	}
}

func TestJSONParser_Instructions_Cached(t *testing.T) {
	p := chat.NewJSONParser[recipe]()
	a := p.Instructions()
	b := p.Instructions()
	if a != b {
		t.Fatal("Instructions must be cached and identical between calls")
	}
}

func TestAnyParser_RejectsMissingFunction(t *testing.T) {
	p := &chat.AnyParser{FormatInstructions: "x"}

	_, err := p.Parse("anything")
	if err == nil {
		t.Fatal("expected error when ParseFunction is nil")
	}
}

func TestWrapParserAsAny_DelegatesParse(t *testing.T) {
	wrapped := chat.WrapParserAsAny(chat.NewListParser())

	got, err := wrapped.Parse("a, b, c")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := got.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", got)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
}

func TestListParserAsAny_MapParserAsAny_JSONParserAsAnyOf(t *testing.T) {
	if list := chat.ListParserAsAny(); list.FormatInstructions == "" {
		t.Fatal("ListParserAsAny instructions empty")
	}
	if m := chat.MapParserAsAny(); m.FormatInstructions == "" {
		t.Fatal("MapParserAsAny instructions empty")
	}
	if j := chat.JSONParserAsAnyOf[recipe](); j.FormatInstructions == "" {
		t.Fatal("JSONParserAsAnyOf instructions empty")
	}
}

func TestRemoveMarkdownCodeBlock_NotPanicOnShort(t *testing.T) {
	// indirect test via MapParser — short inputs go through unchanged.
	p := chat.NewMapParser()
	_, err := p.Parse("ab")
	if err == nil {
		t.Fatal("short non-JSON input should error, not panic")
	}
	if errors.Is(err, errors.New("nothing")) {
		// just sanity-check error chain plumbing
	}
}
