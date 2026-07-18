package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

func TestNewReportsMissingIntegrations(t *testing.T) {
	_, err := New(Config{Sessions: &sessions.Coordinator{}})
	if err == nil || err.Error() != "server: Integrations is required" {
		t.Fatalf("New without Integrations = %v, want named dependency error", err)
	}
}
