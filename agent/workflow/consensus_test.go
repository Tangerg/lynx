package workflow_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

type consensusIn struct{ Question string }
type consensusVote string

func voter(label consensusVote) func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error) {
	return func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error) {
		return label, nil
	}
}

func TestConsensus_PicksMajorityVote(t *testing.T) {
	// 5 voters: 3 say "yes", 2 say "no". Consensus = "yes".
	a, err := workflow.Consensus(workflow.ConsensusConfig[consensusIn, consensusVote]{
		Name: "majority",
		Voters: []func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error){
			voter("yes"), voter("no"), voter("yes"), voter("yes"), voter("no"),
		},
		Key: workflow.DefaultKey[consensusVote],
	})
	if err != nil {
		t.Fatalf("Consensus: %v", err)
	}

	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, runErr := platform.RunAgent(t.Context(), a,
		map[string]any{core.DefaultBindingName: consensusIn{Question: "ok?"}},
		core.ProcessOptions{})
	if runErr != nil {
		t.Fatalf("RunAgent: %v", runErr)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[consensusVote](proc)
	if !ok {
		t.Fatal("no consensusVote bound")
	}
	if got != "yes" {
		t.Fatalf("got %q, want yes", got)
	}
}

func TestConsensus_TieBreakByVoterOrder(t *testing.T) {
	// 2 vs 2 tie; expect the first-seen winner (which was "yes" at idx 0).
	a, err := workflow.Consensus(workflow.ConsensusConfig[consensusIn, consensusVote]{
		Name: "tie",
		Voters: []func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error){
			voter("yes"), voter("no"), voter("yes"), voter("no"),
		},
		Key: workflow.DefaultKey[consensusVote],
	})
	if err != nil {
		t.Fatalf("Consensus: %v", err)
	}
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	mustDeploy(t, platform, a)
	proc, _ := platform.RunAgent(t.Context(), a,
		map[string]any{core.DefaultBindingName: consensusIn{}},
		core.ProcessOptions{})
	got, _ := core.ResultOfType[consensusVote](proc)
	if got != "yes" {
		t.Fatalf("tie should pick first-seen ('yes'), got %q", got)
	}
}

func TestConsensus_RejectsInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		spec workflow.ConsensusConfig[consensusIn, consensusVote]
	}{
		{"empty name", workflow.ConsensusConfig[consensusIn, consensusVote]{
			Voters: []func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error){voter("y")},
			Key:    workflow.DefaultKey[consensusVote],
		}},
		{"empty voters", workflow.ConsensusConfig[consensusIn, consensusVote]{
			Name: "x", Key: workflow.DefaultKey[consensusVote],
		}},
		{"nil key", workflow.ConsensusConfig[consensusIn, consensusVote]{
			Name:   "x",
			Voters: []func(context.Context, *core.ProcessContext, consensusIn) (consensusVote, error){voter("y")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := workflow.Consensus(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
