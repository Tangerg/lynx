package knowledge

import "testing"

func TestTargetDoesNotLeakWorkspaceAcrossScopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     Scope
		workspace string
		wantErr   bool
	}{
		{name: "cwd", scope: WorkingDirectory, workspace: "/repo"},
		{name: "project root", scope: ProjectRoot, workspace: "/repo"},
		{name: "home", scope: Home},
		{name: "cwd without workspace", scope: WorkingDirectory, wantErr: true},
		{name: "home with workspace", scope: Home, workspace: "/repo", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTarget(test.scope, test.workspace)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewTarget() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}
