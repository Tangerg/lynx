package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

type mcpToolStub struct {
	name string
}

func (t mcpToolStub) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name}
}

func (mcpToolStub) Call(context.Context, string) (string, error) {
	return "", nil
}

func TestResolverMCPToolsReadsCurrentPolicy(t *testing.T) {
	disabled := map[string]bool{}
	resolver := &Resolver{mcpToolDisabled: func(name string) bool { return disabled[name] }}
	resolver.SetMCPTools([]chat.Tool{mcpToolStub{name: "files_read"}, mcpToolStub{name: "files_write"}})

	tests := []struct {
		name     string
		disabled map[string]bool
		want     []string
	}{
		{name: "no disabled tools", disabled: map[string]bool{}, want: []string{"files_read", "files_write"}},
		{name: "policy update hides tool", disabled: map[string]bool{"files_write": true}, want: []string{"files_read"}},
		{name: "later policy restores tool", disabled: map[string]bool{}, want: []string{"files_read", "files_write"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disabled = tt.disabled
			gotTools := resolver.mcpTools()
			got := make([]string, len(gotTools))
			for i, tool := range gotTools {
				got[i] = tool.Definition().Name
			}
			if len(got) != len(tt.want) {
				t.Fatalf("tools = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tools = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestResolverSetMCPToolsSnapshotsInput(t *testing.T) {
	resolver := &Resolver{}
	tools := []chat.Tool{mcpToolStub{name: "before"}}
	resolver.SetMCPTools(tools)
	tools[0] = mcpToolStub{name: "after"}

	got := resolver.mcpTools()
	if len(got) != 1 || got[0].Definition().Name != "before" {
		t.Fatalf("mcp tools retained caller-owned slice: %v", got)
	}
}
