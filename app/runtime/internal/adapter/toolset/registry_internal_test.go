package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type invalidSchemaTool struct{}

func (invalidSchemaTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "broken",
		Description: "invalid test tool",
		InputSchema: json.RawMessage(`{`),
	}
}

func (invalidSchemaTool) Call(context.Context, string) (string, error) { return "", nil }

func TestRegistryRejectsInvalidCatalogSchema(t *testing.T) {
	registry := &registry{src: &Resolver{catalog: []tools.Tool{invalidSchemaTool{}}}}
	_, err := registry.List(t.Context())
	if !errors.Is(err, tool.ErrInvalidSchema) {
		t.Fatalf("List error = %v, want ErrInvalidSchema", err)
	}
}
