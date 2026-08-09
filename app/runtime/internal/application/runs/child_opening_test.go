package runs

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestChildOpeningCancellationBeforeClaimPreventsTransaction(t *testing.T) {
	request, confirmation := NewChildOpeningRequest(time.Unix(1, 0))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := confirmation.Await(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Await error = %v, want context cancellation", err)
	}
	if request.claim() {
		t.Fatal("canceled child opening request remained claimable")
	}
}

func TestClaimedChildOpeningWaitsForAuthoritativeResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		request, confirmation := NewChildOpeningRequest(time.Unix(1, 0))
		if !request.claim() {
			t.Fatal("claim child opening request")
		}
		ctx, cancel := context.WithCancel(t.Context())
		type awaitResult struct {
			binding ChildRunBinding
			err     error
		}
		result := make(chan awaitResult, 1)
		go func() {
			binding, err := confirmation.Await(ctx)
			result <- awaitResult{binding: binding, err: err}
		}()
		cancel()
		synctest.Wait()
		select {
		case result := <-result:
			t.Fatalf("Await returned before claimed transaction completed: %+v", result)
		default:
		}

		commitErr := errors.New("child opening commit failed")
		if err := request.complete(ChildRunBinding{}, commitErr); err != nil {
			t.Fatalf("complete: %v", err)
		}
		synctest.Wait()
		if result := <-result; !errors.Is(result.err, commitErr) || result.binding != (ChildRunBinding{}) {
			t.Fatalf("Await result = %+v, want commit error", result)
		}
		binding := ChildRunBinding{MemberID: "member_child", RunID: "run_child", ParentRunID: "run_root"}
		if err := request.complete(binding, nil); err == nil {
			t.Fatal("child opening request completed more than once")
		}
	})
}

func TestChildOpeningRequestRequiresStartTimeAndConfirmation(t *testing.T) {
	request, _ := NewChildOpeningRequest(time.Time{})
	if err := request.validate(); err == nil {
		t.Fatal("zero child start time passed validation")
	}
	if err := (ChildOpeningRequest{StartedAt: time.Unix(1, 0)}).validate(); err == nil {
		t.Fatal("missing child opening confirmation passed validation")
	}
}

func TestInvalidChildOpeningResultFailsCoordinatorAndWaiter(t *testing.T) {
	request, confirmation := NewChildOpeningRequest(time.Unix(1, 0))
	if !request.claim() {
		t.Fatal("claim child opening request")
	}
	if err := request.complete(ChildRunBinding{MemberID: "member_child"}, nil); err == nil {
		t.Fatal("invalid successful binding did not fail the completing Coordinator")
	}
	if binding, err := confirmation.Await(t.Context()); err == nil || binding != (ChildRunBinding{MemberID: "member_child"}) {
		t.Fatalf("confirmation result = (%+v, %v), want the same invalid binding and error", binding, err)
	}

	request, confirmation = NewChildOpeningRequest(time.Unix(2, 0))
	if !request.claim() {
		t.Fatal("claim failed child opening request")
	}
	commitErr := errors.New("commit failed")
	unexpected := ChildRunBinding{MemberID: "member_child", RunID: "run_child", ParentRunID: "run_root"}
	if err := request.complete(unexpected, commitErr); err == nil {
		t.Fatal("failed opening accepted a non-empty binding")
	}
	if binding, err := confirmation.Await(t.Context()); !errors.Is(err, commitErr) || binding != (ChildRunBinding{}) {
		t.Fatalf("failed confirmation result = (%+v, %v), want empty binding and commit error", binding, err)
	}
}

func TestValidateChildRunBindingsRequiresOneConnectedAppTree(t *testing.T) {
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
