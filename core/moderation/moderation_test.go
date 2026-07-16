package moderation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/moderation"
)

func TestModelFunc(t *testing.T) {
	want := errors.New("boom")
	model := moderation.ModelFunc(func(_ context.Context, request *moderation.Request) (*moderation.Response, error) {
		if len(request.Texts) != 1 || request.Texts[0] != "hello" {
			t.Fatalf("texts = %#v", request.Texts)
		}
		return nil, want
	})
	request, _ := moderation.NewRequest([]string{"hello"})
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := moderation.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := moderation.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := moderation.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted empty texts")
	}
	if _, err := moderation.NewRequest([]string{"valid", ""}); err == nil {
		t.Fatal("NewRequest accepted an empty text entry")
	}
	if merged, err := (moderation.Options{}).Merged(); err != nil || merged.Model != "" || len(merged.Extensions) != 0 {
		t.Fatalf("zero Options.Merged() = %#v, %v", merged, err)
	}
	if err := (*moderation.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &moderation.Request{
		Texts:   []string{"text"},
		Options: moderation.Options{Extensions: metadata.Map{"provider/broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = moderation.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	options := new(moderation.Options)
	if err := options.SetExtension("provider/value", func() {}); err == nil || options.Extensions != nil {
		t.Fatalf("failed SetExtension mutated options: %#v, %v", options.Extensions, err)
	}
	if _, err := (moderation.Options{Model: " model "}).Merged(); err == nil {
		t.Fatal("Merged accepted invalid base options")
	}
}

func TestCategoriesAndResponse(t *testing.T) {
	categories := moderation.Categories{"hate": {}}
	if categories.Flagged() {
		t.Fatal("zero Categories is flagged")
	}
	categories["provider/new_category"] = moderation.Verdict{Flagged: true, Score: 0.75}
	if !categories.Flagged() {
		t.Fatal("Flagged did not aggregate Hate")
	}
	result, err := moderation.NewResult(categories, &moderation.ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := moderation.NewResponse([]*moderation.Result{result}, &moderation.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if response.First() != result {
		t.Fatal("First did not return first result")
	}
	categories["provider/new_category"] = moderation.Verdict{}
	if !result.Categories["provider/new_category"].Flagged {
		t.Fatal("NewResult aliases caller categories")
	}
}

func TestOptionsMergeAndCopies(t *testing.T) {
	base := moderation.Options{Model: "base", Extensions: mustMetadata(t, map[string]any{"provider/base": true})}
	merged, err := base.Merged(moderation.Options{
		Model:      "override",
		Extensions: mustMetadata(t, map[string]any{"provider/override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || len(merged.Extensions) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	clone := merged.Clone()
	if err := clone.Extensions.Set("provider/base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, merged.Extensions, "provider/base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	result, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustDecode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := metadata.Decode[T](values, key)
	if err != nil || !ok {
		t.Fatalf("metadata.Decode(%q) = %#v, %t, %v", key, value, ok, err)
	}
	return value
}

func TestCategoriesRemainOpen(t *testing.T) {
	categories := moderation.Categories{"future/provider_category": {Flagged: true, Score: 1}}
	if !categories.Flagged() {
		t.Fatal("provider category was not aggregated")
	}
	if moderation.Categories(nil).Flagged() {
		t.Fatal("nil Categories is flagged")
	}
}

func TestResponseConstructorsRejectInvalidValues(t *testing.T) {
	categories := moderation.Categories{"safe": {}}
	metadata := new(moderation.ResultMetadata)
	if _, err := moderation.NewResult(nil, metadata); err == nil {
		t.Fatal("NewResult accepted empty categories")
	}
	if _, err := moderation.NewResult(categories, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	for name, verdict := range map[string]moderation.Verdict{
		"negative": {Score: -0.1},
		"too high": {Score: 1.1},
	} {
		if _, err := moderation.NewResult(moderation.Categories{name: verdict}, metadata); err == nil {
			t.Fatalf("NewResult accepted %s score", name)
		}
	}
	result, _ := moderation.NewResult(categories, metadata)
	if _, err := moderation.NewResponse(nil, &moderation.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted no results")
	}
	if _, err := moderation.NewResponse([]*moderation.Result{result}, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	if (&moderation.Response{}).First() != nil || (*moderation.Response)(nil).First() != nil {
		t.Fatal("empty response returned a result")
	}
}
