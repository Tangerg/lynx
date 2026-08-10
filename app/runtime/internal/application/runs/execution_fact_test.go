package runs

import "testing"

func TestExecutorMemberValidate(t *testing.T) {
	tests := []struct {
		name    string
		member  ExecutorMember
		wantErr string
	}{
		{
			name: "root executor member",
			member: ExecutorMember{
				MemberID: "member_root",
			},
		},
		{
			name: "child executor member",
			member: ExecutorMember{
				MemberID:    "member_child",
				ParentID:    "member_root",
				SpawnCallID: "call_delegate",
			},
		},
		{
			name: "failure before executor member creation",
		},
		{
			name:    "executor member whitespace",
			member:  ExecutorMember{MemberID: " member_root"},
			wantErr: "runs: executor member id has surrounding whitespace",
		},
		{
			name: "parent whitespace",
			member: ExecutorMember{
				MemberID: "member_child",
				ParentID: "member_root ",
			},
			wantErr: "runs: executor member parent id has surrounding whitespace",
		},
		{
			name: "spawn call whitespace",
			member: ExecutorMember{
				MemberID:    "member_child",
				ParentID:    "member_root",
				SpawnCallID: " call_delegate",
			},
			wantErr: "runs: executor member spawn call id has surrounding whitespace",
		},
		{
			name: "empty executor member with parent",
			member: ExecutorMember{
				ParentID: "member_root",
			},
			wantErr: "runs: empty executor member id cannot carry parent or spawn-call identity",
		},
		{
			name: "empty executor member with spawn call",
			member: ExecutorMember{
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: empty executor member id cannot carry parent or spawn-call identity",
		},
		{
			name: "self-parent",
			member: ExecutorMember{
				MemberID: "member_1",
				ParentID: "member_1",
			},
			wantErr: "runs: executor member cannot parent itself",
		},
		{
			name: "root with spawn call",
			member: ExecutorMember{
				MemberID:    "member_root",
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: root executor member cannot carry spawn-call identity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.member.Validate()
			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("Validate() error = %v, want nil", err)
			case test.wantErr != "" && err == nil:
				t.Fatalf("Validate() error = nil, want %q", test.wantErr)
			case test.wantErr != "" && err.Error() != test.wantErr:
				t.Fatalf("Validate() error = %q, want %q", err, test.wantErr)
			}
		})
	}
}

func TestExecutorEventValidateRequiresPayload(t *testing.T) {
	err := (ExecutorEvent{Member: ExecutorMember{MemberID: "member_root"}}).Validate()
	if err == nil || err.Error() != "runs: executor event payload is required" {
		t.Fatalf("Validate() error = %v, want missing-payload error", err)
	}
}
