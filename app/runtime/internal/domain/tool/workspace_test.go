package tool

import (
	"strconv"
	"testing"
)

func TestBypassImmuneReason_FileMutationScope(t *testing.T) {
	tests := []struct {
		name  string
		scope FileMutationScope
		want  bool
	}{
		{name: "outside", scope: FileMutationOutsideWorkspace, want: true},
		{name: "unknown", scope: FileMutationUnknown, want: true},
		{name: "inside", scope: FileMutationWithinWorkspace},
		{name: "none", scope: FileMutationNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reason, immune := BypassImmuneReason("write", `{}`, test.scope)
			if immune != test.want {
				t.Fatalf("BypassImmuneReason() immune = %v, want %v", immune, test.want)
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
			args := `{"command":` + strconv.Quote(cmd) + `}`
			if _, immune := BypassImmuneReason("shell", args, FileMutationNone); !immune {
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
			args := `{"command":` + strconv.Quote(cmd) + `}`
			if _, immune := BypassImmuneReason("shell", args, FileMutationNone); immune {
				t.Fatalf("ordinary command %q must not be flagged catastrophic", cmd)
			}
		})
	}
}
