package moderation_test

import (
	"context"
	"errors"
	"testing"

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
	if _, err := moderation.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted empty texts")
	}
	if _, err := moderation.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions accepted nil base")
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
	if response.Result() != result {
		t.Fatal("Result did not return first result")
	}
}

func TestOptionsMergeAndProtocolAccessors(t *testing.T) {
	if _, ok := (*moderation.Options)(nil).Get("missing"); ok {
		t.Fatal("nil Options reported a value")
	}
	if _, ok := (*moderation.Request)(nil).Get("missing"); ok {
		t.Fatal("nil Request reported a value")
	}
	if clone := (*moderation.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}

	base := &moderation.Options{Model: "base", Extra: map[string]any{"base": true}}
	merged, err := moderation.MergeOptions(base, nil, &moderation.Options{
		Model: "override",
		Extra: map[string]any{"override": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || len(merged.Extra) != 2 {
		t.Fatalf("MergeOptions = %#v", merged)
	}
	clone := merged.Clone()
	clone.Extra["base"] = false
	if merged.Extra["base"] != true {
		t.Fatal("Options.Clone aliases source state")
	}
	merged.Set("region", "local")
	if value, ok := merged.Get("region"); !ok || value != "local" {
		t.Fatalf("Options.Get = %#v, %t", value, ok)
	}

	request, _ := moderation.NewRequest([]string{"lynx"})
	request.Set("trace_id", "trace-1")
	if value, ok := request.Get("trace_id"); !ok || value != "trace-1" {
		t.Fatalf("Request.Get = %#v, %t", value, ok)
	}
	resultMetadata := &moderation.ResultMetadata{}
	resultMetadata.Set("index", 0)
	if value, ok := resultMetadata.Get("index"); !ok || value != 0 {
		t.Fatalf("ResultMetadata.Get = %#v, %t", value, ok)
	}
	responseMetadata := &moderation.ResponseMetadata{}
	responseMetadata.Set("region", "local")
	if value, ok := responseMetadata.Get("region"); !ok || value != "local" {
		t.Fatalf("ResponseMetadata.Get = %#v, %t", value, ok)
	}
	if _, ok := (*moderation.ResultMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResultMetadata reported a value")
	}
	if _, ok := (*moderation.ResponseMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResponseMetadata reported a value")
	}
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
	if (&moderation.Response{}).Result() != nil || (*moderation.Response)(nil).Result() != nil {
		t.Fatal("empty response returned a result")
	}
}
