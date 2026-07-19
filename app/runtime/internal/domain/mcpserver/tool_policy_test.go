package mcpserver

import "testing"

func TestToolPolicy(t *testing.T) {
	tests := []struct {
		name    string
		servers []Server
		checks  map[ToolRef]struct {
			disabled     bool
			autoApproved bool
		}
	}{
		{
			name: "enabled servers contribute qualified tools",
			servers: []Server{
				{Name: "files", Enabled: true, DisabledTools: []string{"write"}, AutoApproveTools: []string{"read"}},
				{Name: "db", Enabled: true, DisabledTools: []string{"drop"}, AutoApproveTools: []string{"select"}},
			},
			checks: map[ToolRef]struct {
				disabled     bool
				autoApproved bool
			}{
				{Server: "files", Tool: "write"}: {disabled: true},
				{Server: "files", Tool: "read"}:  {autoApproved: true},
				{Server: "db", Tool: "drop"}:     {disabled: true},
				{Server: "db", Tool: "select"}:   {autoApproved: true},
				{Tool: "write"}:                  {},
			},
		},
		{
			name: "disabled servers contribute nothing",
			servers: []Server{
				{Name: "files", Enabled: false, DisabledTools: []string{"write"}, AutoApproveTools: []string{"read"}},
			},
			checks: map[ToolRef]struct {
				disabled     bool
				autoApproved bool
			}{
				{Server: "files", Tool: "write"}: {},
				{Server: "files", Tool: "read"}:  {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewToolPolicy(tt.servers)
			for ref, want := range tt.checks {
				if got := policy.Disabled(ref); got != want.disabled {
					t.Errorf("Disabled(%+v) = %t, want %t", ref, got, want.disabled)
				}
				if got := policy.AutoApproved(ref); got != want.autoApproved {
					t.Errorf("AutoApproved(%+v) = %t, want %t", ref, got, want.autoApproved)
				}
			}
		})
	}
}

func TestZeroToolPolicyDeniesNoTools(t *testing.T) {
	var policy ToolPolicy
	ref := ToolRef{Server: "server", Tool: "tool"}
	if policy.Disabled(ref) || policy.AutoApproved(ref) {
		t.Fatal("zero policy must not disable or auto-approve tools")
	}
}
