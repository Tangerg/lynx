package runs

import "testing"

func TestValidateChildRunBindingsRequiresOneConnectedApplicationTree(t *testing.T) {
	valid := []ChildRunBinding{
		{MemberID: "member_child", RunID: "run_child", ParentRunID: "run_root"},
		{MemberID: "member_grandchild", RunID: "run_grandchild", ParentRunID: "run_child"},
	}
	if err := ValidateChildRunBindings("run_root", valid); err != nil {
		t.Fatalf("valid child Run bindings: %v", err)
	}

	tests := map[string][]ChildRunBinding{
		"duplicate Run": {
			{MemberID: "member_a", RunID: "run_child", ParentRunID: "run_root"},
			{MemberID: "member_b", RunID: "run_child", ParentRunID: "run_root"},
		},
		"duplicate member": {
			{MemberID: "member_child", RunID: "run_a", ParentRunID: "run_root"},
			{MemberID: "member_child", RunID: "run_b", ParentRunID: "run_root"},
		},
		"unknown parent": {
			{MemberID: "member_child", RunID: "run_child", ParentRunID: "run_missing"},
		},
		"cycle": {
			{MemberID: "member_a", RunID: "run_a", ParentRunID: "run_b"},
			{MemberID: "member_b", RunID: "run_b", ParentRunID: "run_a"},
		},
		"root as child": {
			{MemberID: "member_root", RunID: "run_root", ParentRunID: "run_other"},
		},
	}
	for name, bindings := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateChildRunBindings("run_root", bindings); err == nil {
				t.Fatalf("invalid child Run bindings passed validation: %+v", bindings)
			}
		})
	}
}
