package main

import (
	"errors"
	"testing"
)

func TestBootstrapRuntimeRejectsBuildIdentityFailureBeforeExternalSetup(t *testing.T) {
	want := errors.New("executable unreadable")
	_, _, err := bootstrapRuntimeWithBuildID(t.Context(), func() (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("bootstrapRuntimeWithBuildID error = %v, want %v", err, want)
	}
}
