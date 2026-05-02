package id_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document/id"
)

func TestSha256Generator_Deterministic(t *testing.T) {
	gen := id.NewSha256Generator(nil)

	first, err := gen.Generate(context.Background(), "hello", 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gen.Generate(context.Background(), "hello", 42)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same input produced different digests:\n  %s\n  %s", first, second)
	}
}

func TestSha256Generator_DifferentInputsDiffer(t *testing.T) {
	gen := id.NewSha256Generator(nil)
	a, _ := gen.Generate(context.Background(), "hello")
	b, _ := gen.Generate(context.Background(), "world")
	if a == b {
		t.Fatalf("different inputs produced same digest: %s", a)
	}
}

func TestSha256Generator_SaltSeparatesStreams(t *testing.T) {
	plain := id.NewSha256Generator(nil)
	salted := id.NewSha256Generator([]byte("tenant-A"))

	a, _ := plain.Generate(context.Background(), "doc")
	b, _ := salted.Generate(context.Background(), "doc")
	if a == b {
		t.Fatal("salt must change the digest")
	}
}

func TestSha256Generator_EmptyInput(t *testing.T) {
	gen := id.NewSha256Generator(nil)
	got, err := gen.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string for empty input", got)
	}
}

func TestUUIDGenerator_Unique(t *testing.T) {
	gen := id.NewUUIDGenerator()
	first, err := gen.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, _ := gen.Generate(context.Background())
	if first == second {
		t.Fatal("UUID generator returned identical ids")
	}
}

func TestUUIDGenerator_IgnoresInput(t *testing.T) {
	gen := id.NewUUIDGenerator()
	first, _ := gen.Generate(context.Background(), "same input")
	second, _ := gen.Generate(context.Background(), "same input")
	if first == second {
		t.Fatal("UUID must be random regardless of input")
	}
}
