package mcpserver

import "testing"

func TestToolPolicy(t *testing.T) {
	tests := []struct {
		name    string
		servers []Server
		checks  map[string]struct {
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
			checks: map[string]struct {
				disabled     bool
				autoApproved bool
			}{
				"files_write": {disabled: true},
				"files_read":  {autoApproved: true},
				"db_drop":     {disabled: true},
				"db_select":   {autoApproved: true},
				"write":       {},
			},
		},
		{
			name: "disabled servers contribute nothing",
			servers: []Server{
				{Name: "files", Enabled: false, DisabledTools: []string{"write"}, AutoApproveTools: []string{"read"}},
			},
			checks: map[string]struct {
				disabled     bool
				autoApproved bool
			}{
				"files_write": {},
				"files_read":  {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewToolPolicy(tt.servers)
			for tool, want := range tt.checks {
				if got := policy.Disabled(tool); got != want.disabled {
					t.Errorf("Disabled(%q) = %t, want %t", tool, got, want.disabled)
				}
				if got := policy.AutoApproved(tool); got != want.autoApproved {
					t.Errorf("AutoApproved(%q) = %t, want %t", tool, got, want.autoApproved)
				}
			}
		})
	}
}

func TestZeroToolPolicyDeniesNoTools(t *testing.T) {
	var policy ToolPolicy
	if policy.Disabled("server_tool") || policy.AutoApproved("server_tool") {
		t.Fatal("zero policy must not disable or auto-approve tools")
	}
}
