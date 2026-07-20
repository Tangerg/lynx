package core_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestForParallelBranchReturnsClonePanic(t *testing.T) {
	cause := errors.New("clone unavailable")
	process := core.NewProcessContext(core.ProcessContextConfig{
		Blackboard: fakeBlackboard{clone: func() core.Blackboard { panic(cause) }},
	})

	branch, err := process.ForParallelBranch()
	if branch != nil || !errors.Is(err, cause) {
		t.Fatalf("ForParallelBranch = %#v, %v; want nil branch and wrapped clone panic", branch, err)
	}
}

func TestForParallelBranchRejectsNilClone(t *testing.T) {
	process := core.NewProcessContext(core.ProcessContextConfig{
		Blackboard: fakeBlackboard{clone: func() core.Blackboard { return nil }},
	})

	branch, err := process.ForParallelBranch()
	if branch != nil || err == nil {
		t.Fatalf("ForParallelBranch = %#v, %v; want nil branch and error", branch, err)
	}
}
