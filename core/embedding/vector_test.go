package embedding_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
)

func TestFloat32Vector(t *testing.T) {
	if embedding.Float32Vector(nil) != nil {
		t.Fatal("nil vector became non-nil")
	}
	if got, want := embedding.Float32Vector([]float64{1.5, -2.25}), []float32{1.5, -2.25}; !slices.Equal(got, want) {
		t.Fatalf("Float32Vector = %v, want %v", got, want)
	}
}
