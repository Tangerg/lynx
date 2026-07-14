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
