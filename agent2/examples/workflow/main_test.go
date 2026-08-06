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
	const want = "request: ship managed workflow\nreviews: clarity=ready, safety=ready\nprocesses: 4\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
