package tool

import "testing"

func TestMutatesOutsideWorkspace(t *testing.T) {
	const cwd = "/work/project"
	tests := []struct {
		name string
		tool string
		args string
		want bool
	}{
		{"absolute path outside cwd", "write", `{"file_path":"/etc/passwd"}`, true},
		{"absolute path inside cwd", "write", `{"file_path":"/work/project/src/a.go"}`, false},
		{"relative path stays inside", "edit", `{"file_path":"src/a.go"}`, false},
		{"relative path escapes via ..", "edit", `{"file_path":"../../etc/x"}`, true},
		{"home-relative is outside", "download", `{"file_path":"~/secrets"}`, true},
		{"apply_patch outside", "apply_patch", `{"file_path":"/tmp/x"}`, true},
		{"cwd itself is inside", "write", `{"file_path":"/work/project"}`, false},
		{"non-mutating tool never escapes", "read", `{"file_path":"/etc/passwd"}`, false},
		{"shell is not a single-target tool", "shell", `{"command":"rm -rf /etc"}`, false},
		{"missing path arg", "write", `{}`, false},
		{"undecodable args", "write", `{not json`, false},
		{"empty cwd disables the check", "write", `{"file_path":"/etc/passwd"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := cwd
			if tt.name == "empty cwd disables the check" {
				dir = ""
			}
			if got := MutatesOutsideWorkspace(tt.tool, tt.args, dir); got != tt.want {
				t.Fatalf("MutatesOutsideWorkspace(%q, %s, %q) = %v, want %v", tt.tool, tt.args, dir, got, tt.want)
			}
		})
	}
}
