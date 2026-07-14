package tiktoken_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/tokenizer/tiktoken"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tk, err := tiktoken.NewDefault()
	if err != nil {
		t.Fatal(err)
	}

	const want = "hello world"
	encoded, err := tk.Encode(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("encoded token list is empty")
	}

	got, err := tk.Decode(context.Background(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Decode(Encode(%q)) = %q", want, got)
	}
}

func TestEstimateText(t *testing.T) {
	tk, err := tiktoken.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	got, err := tk.EstimateText(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("EstimateText() = %d, want > 0", got)
	}
}

func TestOperationsHonorCanceledContext(t *testing.T) {
	tk, err := tiktoken.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := tk.Encode(ctx, "hello"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Encode() error = %v, want context.Canceled", err)
	}
	if _, err := tk.Decode(ctx, []int{1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Decode() error = %v, want context.Canceled", err)
	}
	if _, err := tk.EstimateText(ctx, "hello"); !errors.Is(err, context.Canceled) {
		t.Fatalf("EstimateText() error = %v, want context.Canceled", err)
	}
}

func TestNewRejectsUnknownEncoding(t *testing.T) {
	if _, err := tiktoken.New("nope-such-encoding"); err == nil {
		t.Fatal("New() accepted an unknown encoding")
	}
}
