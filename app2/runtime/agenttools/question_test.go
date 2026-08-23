package agenttools

import (
	"context"
	"strings"
	"testing"
)

func TestAskUserRejectsNestedWireViolationsBeforeParking(t *testing.T) {
	tool, err := newAskUser("run_test")
	if err != nil {
		t.Fatalf("newAskUser() error = %v", err)
	}

	_, err = tool.Call(context.Background(), `{"fields":[{"prompt":"Pick one","type":"choice","options":[{"label":"Only"}]}]}`)
	if err == nil || !strings.Contains(err.Error(), "options") {
		t.Fatalf("ask_user error = %v, want nested options constraint", err)
	}
}
