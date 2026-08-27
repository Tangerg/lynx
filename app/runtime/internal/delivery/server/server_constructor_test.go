package server

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/sessions"
)

func TestNewReportsMissingIntegrations(t *testing.T) {
	_, err := New(Config{Sessions: &sessions.Coordinator{}})
	if err == nil || err.Error() != "server: MCP is required" {
		t.Fatalf("New without MCP = %v, want named dependency error", err)
	}
}
