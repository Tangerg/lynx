package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type mcpToolStub struct {
	name   string
	server string
	remote string
}

func (t mcpToolStub) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name}
}

func (mcpToolStub) Call(context.Context, string) (string, error) {
	return "", nil
}

func (t mcpToolStub) MCPToolIdentity() (string, string) { return t.server, t.remote }

func TestResolverMCPToolsReadsCurrentPolicy(t *testing.T) {
	disabled := map[mcpserver.ToolRef]bool{}
	resolver := &Resolver{mcpToolDisabled: func(ref mcpserver.ToolRef) bool { return disabled[ref] }}
	resolver.SetMCPTools([]tools.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	})

	tests := []struct {
		name     string
		disabled map[mcpserver.ToolRef]bool
		want     []string
	}{
		{name: "no disabled tools", disabled: map[mcpserver.ToolRef]bool{}, want: []string{"files_read", "files_write"}},
		{
			name:     "policy update hides tool",
			disabled: map[mcpserver.ToolRef]bool{{Server: "files", Tool: "write"}: true},
			want:     []string{"files_read"},
		},
		{name: "later policy restores tool", disabled: map[mcpserver.ToolRef]bool{}, want: []string{"files_read", "files_write"}},
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

func TestResolverMCPPolicyUsesSourceIdentityNotPublicName(t *testing.T) {
	disabledRef := mcpserver.ToolRef{Server: "a_b", Tool: "c"}
	liveRef := mcpserver.ToolRef{Server: "a", Tool: "b_c"}
	if disabledRef.PublicName() != liveRef.PublicName() {
		t.Fatalf("fixture names do not collide: %q != %q", disabledRef.PublicName(), liveRef.PublicName())
	}

	resolver := &Resolver{mcpToolDisabled: func(ref mcpserver.ToolRef) bool { return ref == disabledRef }}
	resolver.SetMCPTools([]tools.Tool{mcpToolStub{
		name: liveRef.PublicName(), server: liveRef.Server, remote: liveRef.Tool,
	}})

	got := resolver.mcpTools()
	if len(got) != 1 || got[0].Definition().Name != liveRef.PublicName() {
		t.Fatalf("policy for %+v hid colliding live tool %+v", disabledRef, liveRef)
	}
}

func TestResolverMCPPolicyFailsClosedWithoutSourceIdentity(t *testing.T) {
	resolver := &Resolver{mcpToolDisabled: func(mcpserver.ToolRef) bool { return false }}
	resolver.SetMCPTools([]tools.Tool{mcpToolStub{name: "missing_identity"}})

	if got := resolver.mcpTools(); len(got) != 0 {
		t.Fatalf("MCP tool without source identity remained visible: %v", got)
	}
}

func TestResolverSetMCPToolsSnapshotsInput(t *testing.T) {
	resolver := &Resolver{}
	tools := []tools.Tool{mcpToolStub{name: "before"}}
	resolver.SetMCPTools(tools)
	tools[0] = mcpToolStub{name: "after"}

	got := resolver.mcpTools()
	if len(got) != 1 || got[0].Definition().Name != "before" {
		t.Fatalf("mcp tools retained caller-owned slice: %v", got)
	}
}
