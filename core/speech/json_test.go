package speech_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/speech"
)

func TestJSONBoundaries(t *testing.T) {
	if _, err := speech.NewOptions(""); !errors.Is(err, speech.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := speech.NewRequest(""); !errors.Is(err, speech.ErrInvalidRequest) {
		t.Fatalf("NewRequest error = %v", err)
	}
	if _, err := speech.NewResponse(nil, &speech.ResponseMetadata{}); !errors.Is(err, speech.ErrInvalidResponse) {
		t.Fatalf("NewResponse error = %v", err)
	}

	if _, err := json.Marshal(speech.Options{Model: " invalid "}); !errors.Is(err, speech.ErrInvalidOptions) {
		t.Fatalf("Marshal Options error = %v", err)
	}
	if _, err := json.Marshal(speech.Request{}); !errors.Is(err, speech.ErrInvalidRequest) {
		t.Fatalf("Marshal Request error = %v", err)
	}
	if _, err := json.Marshal(speech.Response{}); !errors.Is(err, speech.ErrInvalidResponse) {
		t.Fatalf("Marshal Response error = %v", err)
	}

	options := speech.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"model":" invalid "}`), &options); !errors.Is(err, speech.ErrInvalidOptions) {
		t.Fatalf("Unmarshal Options error = %v", err)
	}
	if options.Model != "keep" {
		t.Fatalf("failed Options decode mutated receiver: %#v", options)
	}

	request := speech.Request{Text: "keep"}
	if err := json.Unmarshal([]byte(`{"text":""}`), &request); !errors.Is(err, speech.ErrInvalidRequest) {
		t.Fatalf("Unmarshal Request error = %v", err)
	}
	if request.Text != "keep" {
		t.Fatalf("failed Request decode mutated receiver: %#v", request)
	}

	result, err := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response := speech.Response{
		Result:   result,
		Metadata: &speech.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"result":null,"metadata":{}}`), &response); !errors.Is(err, speech.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.Result != result {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}
}
