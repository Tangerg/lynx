package mcpserver

import (
	"testing"
	"time"
)

func TestServerValidateRejectsNegativeTimeout(t *testing.T) {
	srv := Server{
		Name:      "linear",
		Transport: TransportStreamableHTTP,
		URL:       "https://mcp.linear.app/mcp",
		Timeout:   -time.Second,
	}

	if err := srv.Validate(); err == nil {
		t.Fatal("Validate err = nil, want negative timeout rejected")
	}
}
