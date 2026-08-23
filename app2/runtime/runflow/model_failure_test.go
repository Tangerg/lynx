package runflow

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
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

func TestTerminalProblemIsDurableAndWireStable(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name    string
		outcome rundomain.Outcome
		stored  *protocol.ProblemData
		want    string
	}{
		{
			name: "classified provider failure", outcome: rundomain.Failed,
			stored: &protocol.ProblemData{
				Type: protocol.ProblemProviderUnavailable, Detail: "temporarily unavailable", RetryAfterSeconds: 3,
			},
			want: protocol.ProblemProviderUnavailable,
		},
		{name: "timeout fallback", outcome: rundomain.TimedOut, want: protocol.ProblemTimeout},
		{name: "lost fallback", outcome: rundomain.Lost, want: protocol.ProblemRunLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := rundomain.New(rundomain.Start{
				ID: "run_test", SessionID: "ses_test", SegmentID: "seg_test",
				Provider: "openai-compatible", Model: "test-model", Now: now,
			})
			if err != nil {
				t.Fatalf("run.New() error = %v", err)
			}
			if err := value.Finish("seg_test", test.outcome, "terminal detail", now.Add(time.Second)); err != nil {
				t.Fatalf("Finish() error = %v", err)
			}
			record, err := makeRecord(value, runFacts{
				Metrics: protocol.RunMetrics{}, TerminalError: test.stored,
			})
			if err != nil {
				t.Fatalf("makeRecord() error = %v", err)
			}
			presented, err := presentRecord(record)
			if err != nil {
				t.Fatalf("presentRecord() error = %v", err)
			}
			if presented.Outcome == nil || presented.Outcome.Error == nil || presented.Outcome.Error.Type != test.want {
				t.Fatalf("durable outcome = %+v, want %q", presented.Outcome, test.want)
			}
			if err := protocol.ValidateWireTree(presented); err != nil {
				t.Fatalf("durable Run is not wire-valid: %v", err)
			}
			segment := segmentOutcome(test.outcome, test.stored, "terminal detail")
			if err := protocol.ValidateWireTree(segment); err != nil {
				t.Fatalf("terminal Segment is not wire-valid: %v", err)
			}
		})
	}
}
