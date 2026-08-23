package modelcall

import "testing"

func TestFailureInvariantsAndDiagnostics(t *testing.T) {
	tests := []struct {
		kind       FailureKind
		retryAfter int
		valid      bool
	}{
		{kind: FailureRateLimited, retryAfter: 12, valid: true},
		{kind: FailureUnavailable, retryAfter: 3, valid: true},
		{kind: FailureTimeout, valid: true},
		{kind: FailureRejected, retryAfter: 1, valid: false},
		{kind: "unknown", valid: false},
		{kind: FailureRateLimited, retryAfter: -1, valid: false},
	}
	for _, test := range tests {
		failure, err := NewFailure(test.kind, test.retryAfter)
		if (err == nil) != test.valid {
			t.Errorf("NewFailure(%q, %d) error = %v, valid = %v", test.kind, test.retryAfter, err, test.valid)
			continue
		}
		if test.valid && (!failure.Valid() || failure.Detail() == "") {
			t.Errorf("valid failure = %#v", failure)
		}
	}
}
