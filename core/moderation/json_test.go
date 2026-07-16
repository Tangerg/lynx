package moderation_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/moderation"
)

func TestJSONBoundaries(t *testing.T) {
	if _, err := moderation.NewOptions(""); !errors.Is(err, moderation.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := moderation.NewRequest(nil); !errors.Is(err, moderation.ErrInvalidRequest) {
		t.Fatalf("NewRequest error = %v", err)
	}
	if _, err := moderation.NewResponse(nil, &moderation.ResponseMetadata{}); !errors.Is(err, moderation.ErrInvalidResponse) {
		t.Fatalf("NewResponse error = %v", err)
	}

	if _, err := json.Marshal(moderation.Options{Model: " invalid "}); !errors.Is(err, moderation.ErrInvalidOptions) {
		t.Fatalf("Marshal Options error = %v", err)
	}
	if _, err := json.Marshal(moderation.Request{}); !errors.Is(err, moderation.ErrInvalidRequest) {
		t.Fatalf("Marshal Request error = %v", err)
	}
	if _, err := json.Marshal(moderation.Verdict{Score: 2}); !errors.Is(err, moderation.ErrInvalidResponse) {
		t.Fatalf("Marshal Verdict error = %v", err)
	}
	if _, err := json.Marshal(moderation.Response{}); !errors.Is(err, moderation.ErrInvalidResponse) {
		t.Fatalf("Marshal Response error = %v", err)
	}

	options := moderation.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"model":" invalid "}`), &options); !errors.Is(err, moderation.ErrInvalidOptions) {
		t.Fatalf("Unmarshal Options error = %v", err)
	}
	if options.Model != "keep" {
		t.Fatalf("failed Options decode mutated receiver: %#v", options)
	}

	request := moderation.Request{Texts: []string{"keep"}}
	if err := json.Unmarshal([]byte(`{"texts":[]}`), &request); !errors.Is(err, moderation.ErrInvalidRequest) {
		t.Fatalf("Unmarshal Request error = %v", err)
	}
	if len(request.Texts) != 1 || request.Texts[0] != "keep" {
		t.Fatalf("failed Request decode mutated receiver: %#v", request)
	}

	result, err := moderation.NewResult(
		moderation.Categories{"safe": {}},
		&moderation.ResultMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	response := moderation.Response{
		Results:  []*moderation.Result{result},
		Metadata: &moderation.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"results":[],"metadata":{}}`), &response); !errors.Is(err, moderation.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.First() != result {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}

	verdict := moderation.Verdict{Score: 0.25}
	if err := json.Unmarshal([]byte(`{"score":2}`), &verdict); !errors.Is(err, moderation.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Verdict error = %v", err)
	}
	if verdict.Score != 0.25 {
		t.Fatalf("failed Verdict decode mutated receiver: %#v", verdict)
	}
}
