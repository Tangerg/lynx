package runs

import "testing"

func TestExecutorSourceValidate(t *testing.T) {
	tests := []struct {
		name    string
		source  ExecutorSource
		wantErr string
	}{
		{
			name: "root process",
			source: ExecutorSource{
				ProcessID: "process_root",
			},
		},
		{
			name: "child process",
			source: ExecutorSource{
				ProcessID:   "process_child",
				ParentID:    "process_root",
				SpawnCallID: "call_delegate",
			},
		},
		{
			name: "failure before process creation",
		},
		{
			name:    "process whitespace",
			source:  ExecutorSource{ProcessID: " process_root"},
			wantErr: "runs: executor source process id has surrounding whitespace",
		},
		{
			name: "parent whitespace",
			source: ExecutorSource{
				ProcessID: "process_child",
				ParentID:  "process_root ",
			},
			wantErr: "runs: executor source parent id has surrounding whitespace",
		},
		{
			name: "spawn call whitespace",
			source: ExecutorSource{
				ProcessID:   "process_child",
				ParentID:    "process_root",
				SpawnCallID: " call_delegate",
			},
			wantErr: "runs: executor source spawn call id has surrounding whitespace",
		},
		{
			name: "empty process with parent",
			source: ExecutorSource{
				ParentID: "process_root",
			},
			wantErr: "runs: empty executor process id cannot carry parent or spawn-call identity",
		},
		{
			name: "empty process with spawn call",
			source: ExecutorSource{
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: empty executor process id cannot carry parent or spawn-call identity",
		},
		{
			name: "self-parent",
			source: ExecutorSource{
				ProcessID: "process_1",
				ParentID:  "process_1",
			},
			wantErr: "runs: executor source cannot parent itself",
		},
		{
			name: "root with spawn call",
			source: ExecutorSource{
				ProcessID:   "process_root",
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: root executor source cannot carry spawn-call identity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.source.Validate()
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
	err := (ExecutorEvent{Source: ExecutorSource{ProcessID: "process_root"}}).Validate()
	if err == nil || err.Error() != "runs: executor event payload is required" {
		t.Fatalf("Validate() error = %v, want missing-payload error", err)
	}
}
