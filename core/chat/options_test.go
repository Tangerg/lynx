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

func TestOptionsClone(t *testing.T) {
	options := chat.Options{
		Model:            "model",
		FrequencyPenalty: new(0.1),
		MaxTokens:        new(int64(10)),
		PresencePenalty:  new(0.2),
		Stop:             []string{"END"},
		Temperature:      new(0.3),
		TopK:             new(int64(4)),
		TopP:             new(0.9),
	}
	clone := options.Clone()

	*clone.FrequencyPenalty = 1
	*clone.MaxTokens = 20
	*clone.PresencePenalty = 1
	clone.Stop[0] = "MUTATED"
	*clone.Temperature = 1
	*clone.TopK = 8
	*clone.TopP = 0.5

	if *options.FrequencyPenalty != 0.1 ||
		*options.MaxTokens != 10 ||
		*options.PresencePenalty != 0.2 ||
		options.Stop[0] != "END" ||
		*options.Temperature != 0.3 ||
		*options.TopK != 4 ||
		*options.TopP != 0.9 {
		t.Fatalf("clone mutated source options: %+v", options)
	}
}

func TestOptionsOverlay(t *testing.T) {
	base := chat.Options{
		Model:            "base-model",
		FrequencyPenalty: new(0.1),
		MaxTokens:        new(int64(10)),
		PresencePenalty:  new(0.2),
		Stop:             []string{"BASE"},
		Temperature:      new(0.3),
		TopK:             new(int64(4)),
		TopP:             new(0.9),
	}

	if got := base.Overlay(chat.Options{}); !reflect.DeepEqual(got, base) {
		t.Fatalf("Overlay(empty) = %#v, want unchanged %#v", got, base)
	}

	override := chat.Options{
		Model:       "override-model",
		MaxTokens:   new(int64(20)),
		Stop:        []string{"OVERRIDE"},
		Temperature: new(0.7),
	}
	got := base.Overlay(override)
	if got.Model != "override-model" || *got.MaxTokens != 20 ||
		got.Stop[0] != "OVERRIDE" || *got.Temperature != 0.7 {
		t.Fatalf("Overlay did not apply set fields: %#v", got)
	}
	if *got.FrequencyPenalty != 0.1 || *got.PresencePenalty != 0.2 ||
		*got.TopK != 4 || *got.TopP != 0.9 {
		t.Fatalf("Overlay dropped base fields the override left unset: %#v", got)
	}

	*got.MaxTokens = 99
	got.Stop[0] = "MUTATED"
	*got.FrequencyPenalty = 99
	if *override.MaxTokens != 20 || override.Stop[0] != "OVERRIDE" {
		t.Fatalf("Overlay aliased the override: %#v", override)
	}
	if *base.FrequencyPenalty != 0.1 || base.Stop[0] != "BASE" {
		t.Fatalf("Overlay aliased the base: %#v", base)
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
