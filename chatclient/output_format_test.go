package chatclient

import (
	"encoding/json"
	"errors"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

type recipe struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

func TestOutputFormatContracts(t *testing.T) {
	text := Text()
	if err := text.validate(); err != nil || text.contract.Type != chat.OutputFormatText {
		t.Fatalf("Text = (%#v, %v)", text.contract, err)
	}
	jsonFormat := JSON[recipe]()
	if err := jsonFormat.validate(); err != nil || jsonFormat.contract.Type != chat.OutputFormatJSON {
		t.Fatalf("JSON = (%#v, %v)", jsonFormat.contract, err)
	}
	schema := json.RawMessage(`{"type":"object"}`)
	schemaFormat, err := JSONSchema[recipe]("recipe", schema)
	if err != nil {
		t.Fatal(err)
	}
	contract := schemaFormat.contract.Clone()
	schema[0] = '['
	if contract.Type != chat.OutputFormatJSONSchema || contract.Name != "recipe" || contract.Schema[0] != '{' {
		t.Fatalf("JSONSchema contract = %#v", contract)
	}
	contract.Schema[0] = '['
	if schemaFormat.contract.Schema[0] != '{' {
		t.Fatal("Contract returned aliased schema bytes")
	}
	if _, err := JSONSchema[recipe]("", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("invalid JSONSchema error = %v", err)
	}
	if err := (OutputFormat[recipe]{}).validate(); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("zero OutputFormat error = %v", err)
	}
}

func TestOutputFormatDecodeUsesOneStreamPath(t *testing.T) {
	format := JSON[recipe]()
	want := recipe{Name: "tea", Steps: []string{"boil", "steep"}}

	complete := responseWithText(t, `{"name":"tea","steps":["boil","steep"]}`)
	got, err := format.decode(once(complete, nil))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode complete = (%#v, %v), want %#v", got, err, want)
	}

	stream := iter.Seq2[*chat.Response, error](func(yield func(*chat.Response, error) bool) {
		for _, chunk := range []string{"```j", "son\n{\"name\":\"tea\",", "\"steps\":[\"boil\",\"steep\"]}\n", "```"} {
			if !yield(responseWithText(t, chunk), nil) {
				return
			}
		}
	})
	got, err = format.decode(stream)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode stream = (%#v, %v), want %#v", got, err, want)
	}
}

func TestOutputFormatDecodeIsIndependentOfStreamChunking(t *testing.T) {
	raw := "```json\n{\"name\":\"龙井\",\"steps\":[\"boil\",\"steep\"]}\n```"
	want := recipe{Name: "龙井", Steps: []string{"boil", "steep"}}
	format := JSON[recipe]()

	for split := 0; split <= len(raw); split++ {
		stream := iter.Seq2[*chat.Response, error](func(yield func(*chat.Response, error) bool) {
			if split > 0 && !yield(responseWithText(t, raw[:split]), nil) {
				return
			}
			if split < len(raw) {
				yield(responseWithText(t, raw[split:]), nil)
			}
		})
		got, err := format.decode(stream)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("split %d: Decode = (%#v, %v), want %#v", split, got, err, want)
		}
	}
}

func TestOutputFormatDecodeRobustJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want recipe
	}{
		{name: "plain", raw: `{"name":"tea","steps":[]}`, want: recipe{Name: "tea", Steps: []string{}}},
		{name: "fenced", raw: "```JSON\n{\"name\":\"tea\",\"steps\":[]}\n```", want: recipe{Name: "tea", Steps: []string{}}},
		{name: "surrounding prose", raw: `Here: {"name":"tea","steps":[]} done.`, want: recipe{Name: "tea", Steps: []string{}}},
		{name: "control character", raw: "{\"name\":\"green\ntea\",\"steps\":[]}", want: recipe{Name: "green\ntea", Steps: []string{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := JSON[recipe]().decode(once(responseWithText(t, test.raw), nil))
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Decode = (%#v, %v), want %#v", got, err, test.want)
			}
		})
	}
}

func TestOutputFormatDecodeRejectsLossyOrAmbiguousJSON(t *testing.T) {
	for _, raw := range []string{
		`{"name":"tea","steps":[]`,
		`{"name":"first","steps":[]} {"name":"second","steps":[]}`,
		`{"name":"tea","name":"coffee","steps":[]}`,
		string([]byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', ',', '"', 's', 't', 'e', 'p', 's', '"', ':', '[', ']', '}'}),
	} {
		if _, err := JSON[recipe]().decode(once(responseWithText(t, raw), nil)); err == nil {
			t.Fatalf("Decode(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestOutputFormatDecodeBoundaries(t *testing.T) {
	upstream := errors.New("upstream")
	format := JSON[recipe]()
	if _, err := format.decode(nil); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("nil sequence error = %v", err)
	}
	if _, err := format.decode(once(nil, nil)); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("nil response error = %v", err)
	}
	if _, err := format.decode(once(nil, upstream)); !errors.Is(err, upstream) {
		t.Fatalf("upstream error = %v", err)
	}
	empty := iter.Seq2[*chat.Response, error](func(func(*chat.Response, error) bool) {})
	if _, err := format.decode(empty); !errors.Is(err, ErrInvalidOutputFormat) {
		t.Fatalf("empty sequence error = %v", err)
	}
	if got, err := Text().decode(once(responseWithText(t, " exact \n"), nil)); err != nil || got != " exact \n" {
		t.Fatalf("text Decode = (%q, %v)", got, err)
	}
}

func responseWithText(t *testing.T, text string) *chat.Response {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, err := chat.NewResponse(&chat.Result{Message: &message}, &chat.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	return response
}
