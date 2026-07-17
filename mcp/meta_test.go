package mcp_test

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

func TestMetaContextOwnsTopLevelMap(t *testing.T) {
	meta := sdkmcp.Meta{"requestId": "original"}
	ctx := lynxmcp.WithMeta(t.Context(), meta)
	meta["requestId"] = "caller mutation"

	first := lynxmcp.MetaFromContext(ctx)
	if got := first["requestId"]; got != "original" {
		t.Fatalf("MetaFromContext requestId = %v, want original", got)
	}
	first["requestId"] = "consumer mutation"

	if got := lynxmcp.MetaFromContext(ctx)["requestId"]; got != "original" {
		t.Fatalf("second MetaFromContext requestId = %v, want original", got)
	}
}
