package chat_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func TestOptionsZeroValueIsValid(t *testing.T) {
	var options chat.Options
	if err := options.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{}` {
		t.Fatalf("Options{} JSON = %s, want {}", encoded)
	}
}

func TestOptionsValidateBoundaries(t *testing.T) {
	options := chat.Options{
		Model:            "model",
		FrequencyPenalty: new(-2.0),
		MaxTokens:        new(int64(1)),
		PresencePenalty:  new(2.0),
		Stop:             []string{"stop"},
		Temperature:      new(0.0),
		TopK:             new(int64(1)),
		TopP:             new(1.0),
	}
	if err := options.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	var got chat.Options
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, options) {
		t.Fatalf("round trip = %#v, want %#v", got, options)
	}
}

func TestOptionsValidateRejectsInvalidOverrides(t *testing.T) {
	tests := []struct {
		name    string
		options chat.Options
	}{
		{name: "model whitespace", options: chat.Options{Model: " model"}},
		{name: "frequency low", options: chat.Options{FrequencyPenalty: new(-2.1)}},
		{name: "frequency NaN", options: chat.Options{FrequencyPenalty: new(math.NaN())}},
		{name: "max tokens zero", options: chat.Options{MaxTokens: new(int64(0))}},
		{name: "presence high", options: chat.Options{PresencePenalty: new(2.1)}},
		{name: "empty stop", options: chat.Options{Stop: []string{""}}},
		{name: "temperature high", options: chat.Options{Temperature: new(2.1)}},
		{name: "temperature infinity", options: chat.Options{Temperature: new(math.Inf(1))}},
		{name: "top k zero", options: chat.Options{TopK: new(int64(0))}},
		{name: "top p high", options: chat.Options{TopP: new(1.1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.options.Validate(); !errors.Is(err, chat.ErrInvalidOptions) {
				t.Fatalf("Validate error = %v, want ErrInvalidOptions", err)
			}
			if _, err := json.Marshal(tt.options); !errors.Is(err, chat.ErrInvalidOptions) {
				t.Fatalf("Marshal error = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

func TestOptionsUnmarshalIsAtomic(t *testing.T) {
	got := chat.Options{Model: "keep"}
	if err := json.Unmarshal([]byte(`{"temperature":3}`), &got); !errors.Is(err, chat.ErrInvalidOptions) {
		t.Fatalf("Unmarshal error = %v, want ErrInvalidOptions", err)
	}
	if got.Model != "keep" {
		t.Fatalf("failed Unmarshal mutated receiver: %+v", got)
	}
}

func TestOptionsNilUnmarshalReceiver(t *testing.T) {
	var options *chat.Options
	if err := options.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, chat.ErrInvalidOptions) {
		t.Fatalf("UnmarshalJSON error = %v, want ErrInvalidOptions", err)
	}
}
