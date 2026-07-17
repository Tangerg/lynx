package tool

import "testing"

func TestBypassImmuneReason_OutOfWorkspaceFileMutation(t *testing.T) {
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
			reason, immune := BypassImmuneReason(tt.tool, tt.args, dir)
			if immune != tt.want {
				t.Fatalf("BypassImmuneReason(%q, %s, %q) immune = %v, want %v", tt.tool, tt.args, dir, immune, tt.want)
			}
			if immune && reason == "" {
				t.Fatal("an immune call must carry a reason")
			}
		})
	}
}

func TestBypassImmuneReason_CatastrophicShell(t *testing.T) {
	flag := []string{ // catastrophic — must confirm even in bypass
		`rm -rf /`,
		`rm -rf ~`,
		`rm -rf $HOME`,
		`rm -rf ${HOME}`,
		`rm -rf /*`,
		`rm -fr ~/`,
		`rm -r -f /`,
		`sudo rm -rf /`,
		`rm --recursive --force /`,
		`rm -rf --no-preserve-root /`,
		`cd /tmp && rm -rf ~`,
		`:(){ :|:& };:`,
		`mkfs.ext4 /dev/sda1`,
		`dd if=/dev/zero of=/dev/sda`,
		`echo x > /dev/sda`,
	}
	for _, cmd := range flag {
		t.Run("flag:"+cmd, func(t *testing.T) {
			args := `{"command":` + quote(cmd) + `}`
			if _, immune := BypassImmuneReason("shell", args, "/work"); !immune {
				t.Fatalf("command %q should be catastrophic (immune), was not", cmd)
			}
		})
	}

	allow := []string{ // ordinary — must NOT trip the confirm
		`rm -rf ./build`,
		`rm -rf node_modules`,
		`rm -rf /tmp/scratch-dir`,
		`rm file.txt`,
		`ls -la`,
		`git clean -fdx`,
		`echo "rm -rf /" >> notes.txt`, // mentions it in a string, doesn't run it against root
		`dd if=in.img of=out.img`,
	}
	for _, cmd := range allow {
		t.Run("allow:"+cmd, func(t *testing.T) {
			args := `{"command":` + quote(cmd) + `}`
			if _, immune := BypassImmuneReason("shell", args, "/work"); immune {
				t.Fatalf("ordinary command %q must not be flagged catastrophic", cmd)
			}
		})
	}
}

// quote JSON-encodes a command string for embedding in a tool arguments blob.
func quote(s string) string {
	var b []byte
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b = append(b, '\\', byte(r))
		default:
			b = append(b, string(r)...)
		}
	}
	b = append(b, '"')
	return string(b)
}
