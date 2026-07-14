package tokenizer_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/tokenizer"
)

func TestTiktoken_EncodeDecode_RoundTrip(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	want := "hello world"
	encoded, err := tk.Encode(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("encoded is empty")
	}

	decoded, err := tk.Decode(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != want {
		t.Fatalf("round-trip lost data: got %q, want %q", decoded, want)
	}
}

func TestTiktoken_EstimateText(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}

	got, err := tk.EstimateText(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("token count = %d, want > 0", got)
	}
}

func TestTiktoken_EstimateMedia_Empty(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}
	got, err := tk.EstimateMedia(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestTiktoken_EstimateMedia_URI(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := media.NewURI("text/plain", "https://example.com/hello.txt")

	got, err := tk.EstimateMedia(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("token count = %d, want > 0", got)
	}
}

func TestTiktoken_EstimateMedia_Bytes(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := media.NewBytes("application/octet-stream", []byte("payload"))

	got, err := tk.EstimateMedia(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("token count = %d, want > 0", got)
	}
}

func TestTiktoken_EstimateMedia_Reference(t *testing.T) {
	tk, err := tokenizer.NewDefaultTiktoken()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := media.NewReference("application/json", "provider-file-42")

	got, err := tk.EstimateMedia(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("token count = %d, want > 0", got)
	}
}

func TestNewTiktoken_UnknownEncoding(t *testing.T) {
	if _, err := tokenizer.NewTiktoken("nope-such-encoding"); err == nil {
		t.Fatal("unknown encoding must error")
	}
}
