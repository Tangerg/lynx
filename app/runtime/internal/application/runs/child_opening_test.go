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

	if err := confirmation.Await(ctx); !errors.Is(err, context.Canceled) {
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
		result := make(chan error, 1)
		go func() {
			result <- confirmation.Await(ctx)
		}()
		cancel()
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("Await returned before claimed transaction completed: %v", err)
		default:
		}

		commitErr := errors.New("child opening commit failed")
		if err := request.complete(commitErr); err != nil {
			t.Fatalf("complete: %v", err)
		}
		synctest.Wait()
		if err := <-result; !errors.Is(err, commitErr) {
			t.Fatalf("Await error = %v, want commit error", err)
		}
		if err := request.complete(nil); err == nil {
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
