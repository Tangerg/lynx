package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "20 + 22 = 42\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
