package transcription_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/transcription"
)

func TestJSONBoundaries(t *testing.T) {
	if _, err := transcription.NewOptions(""); !errors.Is(err, transcription.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := transcription.NewRequest(nil); !errors.Is(err, transcription.ErrInvalidRequest) {
		t.Fatalf("NewRequest error = %v", err)
	}
	if _, err := transcription.NewResponse(nil, &transcription.ResponseMetadata{}); !errors.Is(err, transcription.ErrInvalidResponse) {
		t.Fatalf("NewResponse error = %v", err)
	}
	var extensionOptions transcription.Options
	if err := extensionOptions.SetExtension("invalid", true); !errors.Is(err, transcription.ErrInvalidOptions) {
		t.Fatalf("SetExtension error = %v", err)
	}

	if _, err := json.Marshal(transcription.Options{Model: " invalid "}); !errors.Is(err, transcription.ErrInvalidOptions) {
		t.Fatalf("Marshal Options error = %v", err)
	}
	if _, err := json.Marshal(transcription.Request{}); !errors.Is(err, transcription.ErrInvalidRequest) {
		t.Fatalf("Marshal Request error = %v", err)
	}
	if _, err := json.Marshal(transcription.Response{}); !errors.Is(err, transcription.ErrInvalidResponse) {
		t.Fatalf("Marshal Response error = %v", err)
	}

	options := transcription.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"model":" invalid "}`), &options); !errors.Is(err, transcription.ErrInvalidOptions) {
		t.Fatalf("Unmarshal Options error = %v", err)
	}
	if options.Model != "keep" {
		t.Fatalf("failed Options decode mutated receiver: %#v", options)
	}

	audio, err := media.NewBytes("audio/mpeg", []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	request := transcription.Request{Audio: audio}
	if err := json.Unmarshal([]byte(`{"audio":null}`), &request); !errors.Is(err, transcription.ErrInvalidRequest) {
		t.Fatalf("Unmarshal Request error = %v", err)
	}
	if request.Audio != audio {
		t.Fatalf("failed Request decode mutated receiver: %#v", request)
	}

	result, err := transcription.NewResult("keep", &transcription.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response := transcription.Response{
		Result:   result,
		Metadata: &transcription.ResponseMetadata{},
	}
	if err := json.Unmarshal([]byte(`{"result":null,"metadata":{}}`), &response); !errors.Is(err, transcription.ErrInvalidResponse) {
		t.Fatalf("Unmarshal Response error = %v", err)
	}
	if response.Result != result {
		t.Fatalf("failed Response decode mutated receiver: %#v", response)
	}
}
