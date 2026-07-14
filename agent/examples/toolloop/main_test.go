package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	const want = "tool add => 5\nassistant => 5\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
