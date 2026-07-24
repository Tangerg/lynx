package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func TestStartTurnRequestSnapshotOwnsProtocolValues(t *testing.T) {
	t.Parallel()

	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("media.NewBytes: %v", err)
	}
	temperature := 0.7
	request := StartTurnRequest{
		SessionID:      "session",
		Message:        "hello",
		Media:          []*media.Media{image},
		Options:        &corechat.Options{Temperature: &temperature, Stop: []string{"done"}},
		InterruptKinds: []runs.InterruptKind{runs.ApprovalInterruptKind},
	}

	snapshot := request.snapshot()
	*request.Options.Temperature = 1.4
	request.Options.Stop[0] = "changed"
	request.Media[0].Source.Bytes[0] = 9
	request.Media[0] = nil
	request.InterruptKinds[0] = runs.QuestionInterruptKind

	if snapshot.Options == nil || snapshot.Options.Temperature == nil || *snapshot.Options.Temperature != 0.7 {
		t.Fatalf("snapshot temperature = %+v, want 0.7", snapshot.Options)
	}
	if got := snapshot.Options.Stop; len(got) != 1 || got[0] != "done" {
		t.Fatalf("snapshot stop = %v, want [done]", got)
	}
	if len(snapshot.Media) != 1 || snapshot.Media[0] == nil || snapshot.Media[0].Source.Bytes[0] != 1 {
		t.Fatalf("snapshot media = %+v, want independent image bytes", snapshot.Media)
	}
	if got := snapshot.InterruptKinds; len(got) != 1 || got[0] != "approval" {
		t.Fatalf("snapshot interrupt kinds = %v, want [approval]", got)
	}
}
