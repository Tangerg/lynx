package modelboundary

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
)

func TestCarrierRoundTripIsSecretFreeAndPreservesLocalCause(t *testing.T) {
	cause := errors.New("provider response contains secret material")
	failure, err := modelcall.NewFailure(modelcall.FailureRateLimited, 12)
	if err != nil {
		t.Fatal(err)
	}
	carried := Carry(failure, cause)
	if !errors.Is(carried, cause) {
		t.Fatal("carrier lost its local error chain")
	}
	if carried.Error() == cause.Error() {
		t.Fatal("carrier exposed the provider diagnostic")
	}
	decoded, ok := Decode(carried.Error())
	if !ok || decoded.Kind() != modelcall.FailureRateLimited || decoded.RetryAfterSeconds() != 12 {
		t.Fatalf("decoded failure = %#v, %v", decoded, ok)
	}
}

func TestDecodeRejectsForeignAndExtendedDiagnostics(t *testing.T) {
	if _, ok := Decode("provider failed"); ok {
		t.Fatal("accepted a foreign diagnostic")
	}
	failure, _ := modelcall.NewFailure(modelcall.FailureTimeout, 0)
	if _, ok := Decode(Carry(failure, nil).Error() + "trailing"); ok {
		t.Fatal("accepted an extended carrier")
	}
}
