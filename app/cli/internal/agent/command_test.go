package agent

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestCommandIDHasAStableValidatedWireIdentity(t *testing.T) {
	id, err := newCommandID(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(id), "cli_00000000000000000000000000000000"; got != want {
		t.Fatalf("command id = %q, want %q", got, want)
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("generated command id is invalid: %v", err)
	}
	for _, invalid := range []CommandID{"", "other_00000000000000000000000000000000", "cli_short", "cli_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid command id %q was accepted", invalid)
		}
	}
}

func TestCommandIDReportsEntropyFailure(t *testing.T) {
	if _, err := newCommandID(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty entropy error = %v, want EOF", err)
	}
}
