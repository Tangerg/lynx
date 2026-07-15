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
	if _, err := (*moderation.Options)(nil).Merged(); err == nil {
		t.Fatal("Merged accepted nil receiver")
	}
	if err := (*moderation.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &moderation.Request{
		Texts:   []string{"text"},
		Options: &moderation.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &moderation.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	options := new(moderation.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
}

func TestCategoriesAndResponse(t *testing.T) {
	categories := &moderation.Categories{}
	if categories.Flagged() {
		t.Fatal("zero Categories is flagged")
	}
	categories.Hate.Flagged = true
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
}

func TestOptionsMergeAndCopies(t *testing.T) {
	if clone := (*moderation.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}

	base := &moderation.Options{Model: "base", Extra: mustMetadata(t, map[string]any{"base": true})}
	merged, err := base.Merged(nil, &moderation.Options{
		Model: "override",
		Extra: mustMetadata(t, map[string]any{"override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || len(merged.Extra) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	clone := merged.Clone()
	if err := clone.Extra.Set("base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, merged.Extra, "base") {
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

func TestCategoriesFlaggedCoversEveryDimension(t *testing.T) {
	setters := []func(*moderation.Categories){
		func(c *moderation.Categories) { c.Sexual.Flagged = true },
		func(c *moderation.Categories) { c.Hate.Flagged = true },
		func(c *moderation.Categories) { c.Harassment.Flagged = true },
		func(c *moderation.Categories) { c.SelfHarm.Flagged = true },
		func(c *moderation.Categories) { c.SexualMinors.Flagged = true },
		func(c *moderation.Categories) { c.HateThreatening.Flagged = true },
		func(c *moderation.Categories) { c.ViolenceGraphic.Flagged = true },
		func(c *moderation.Categories) { c.SelfHarmIntent.Flagged = true },
		func(c *moderation.Categories) { c.SelfHarmInstructions.Flagged = true },
		func(c *moderation.Categories) { c.HarassmentThreatening.Flagged = true },
		func(c *moderation.Categories) { c.Violence.Flagged = true },
		func(c *moderation.Categories) { c.DangerousAndCriminalContent.Flagged = true },
		func(c *moderation.Categories) { c.Health.Flagged = true },
		func(c *moderation.Categories) { c.Financial.Flagged = true },
		func(c *moderation.Categories) { c.Law.Flagged = true },
		func(c *moderation.Categories) { c.Pii.Flagged = true },
		func(c *moderation.Categories) { c.Illicit.Flagged = true },
		func(c *moderation.Categories) { c.IllicitViolent.Flagged = true },
	}
	for index, set := range setters {
		categories := new(moderation.Categories)
		set(categories)
		if !categories.Flagged() {
			t.Fatalf("category %d was not aggregated", index)
		}
	}
	if (*moderation.Categories)(nil).Flagged() {
		t.Fatal("nil Categories is flagged")
	}
}

func TestResponseConstructorsRejectInvalidValues(t *testing.T) {
	categories := new(moderation.Categories)
	metadata := new(moderation.ResultMetadata)
	if _, err := moderation.NewResult(nil, metadata); err == nil {
		t.Fatal("NewResult accepted nil categories")
	}
	if _, err := moderation.NewResult(categories, nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
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
