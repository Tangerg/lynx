package agentexec

import (
	"context"
	"encoding/json"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

type deferringSearchTool struct{}

func (deferringSearchTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "search_tools", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (deferringSearchTool) Call(context.Context, string) (string, error) { return "", nil }

func (deferringSearchTool) DeferredToolNames() []string { return []string{"catalog_a"} }

type catalogTool struct{ name string }

func (c catalogTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: c.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (catalogTool) Call(context.Context, string) (string, error) { return "", nil }

// TestDeferredManifestSurvivesObservation pins the real seam: every resolved
// tool reaches the turn already wrapped for observation, so a manifest built
// from those wrappers must still withhold what the search tool defers. Losing
// it would advertise the whole catalog the deferral exists to hide, silently.
func TestDeferredManifestSurvivesObservation(t *testing.T) {
	middleware := &toolObserverMiddleware{observation: newToolObservation(noopObserver{}, nil, 0, "")}
	observed := []toolcontract.Tool{
		middleware.WrapTool(nil, core.ActionDescriptor{}, deferringSearchTool{}),
		middleware.WrapTool(nil, core.ActionDescriptor{}, catalogTool{name: "catalog_a"}),
		middleware.WrapTool(nil, core.ActionDescriptor{}, catalogTool{name: "read"}),
	}

	manifest, err := toolloop.InitialManifest(observed)
	if err != nil {
		t.Fatalf("InitialManifest: %v", err)
	}
	var advertised []string
	for _, definition := range manifest {
		advertised = append(advertised, definition.Name)
	}
	if len(advertised) != 2 || advertised[0] != "read" || advertised[1] != "search_tools" {
		t.Fatalf("advertised = %v, want the deferred catalog tool withheld", advertised)
	}
}
