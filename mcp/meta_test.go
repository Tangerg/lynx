package mcp_test

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	scopemcp "github.com/Tangerg/scope/mcp"
)

func TestMetaContextOwnsTopLevelMap(t *testing.T) {
	meta := sdkmcp.Meta{"requestId": "original"}
	ctx := scopemcp.WithMeta(t.Context(), meta)
	meta["requestId"] = "caller mutation"

	first := scopemcp.MetaFromContext(ctx)
	if got := first["requestId"]; got != "original" {
		t.Fatalf("MetaFromContext requestId = %v, want original", got)
	}
	first["requestId"] = "consumer mutation"

	if got := scopemcp.MetaFromContext(ctx)["requestId"]; got != "original" {
		t.Fatalf("second MetaFromContext requestId = %v, want original", got)
	}
}
