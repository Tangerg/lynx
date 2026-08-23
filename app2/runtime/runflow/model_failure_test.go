package runflow

import (
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestModelFailureProjectionUsesLyraProblemVocabulary(t *testing.T) {
	tests := []struct {
		kind       modelcall.FailureKind
		retryAfter int
		want       string
	}{
		{modelcall.FailureRateLimited, 12, protocol.ProblemRateLimited},
		{modelcall.FailureInvalidCredentials, 0, protocol.ProblemInvalidAPIKey},
		{modelcall.FailureTimeout, 0, protocol.ProblemTimeout},
		{modelcall.FailureUnavailable, 3, protocol.ProblemProviderUnavailable},
		{modelcall.FailureRejected, 0, protocol.ProblemProviderRejected},
	}
	for _, test := range tests {
		failure, err := modelcall.NewFailure(test.kind, test.retryAfter)
		if err != nil {
			t.Fatal(err)
		}
		problem := modelFailureProblem(&failure)
		if problem == nil || problem.Type != test.want || problem.Detail == "" || problem.RetryAfterSeconds != test.retryAfter {
			t.Errorf("project %q = %#v", test.kind, problem)
		}
	}
	if problem := modelFailureProblem(nil); problem != nil {
		t.Fatalf("nil failure projected as %#v", problem)
	}
}
