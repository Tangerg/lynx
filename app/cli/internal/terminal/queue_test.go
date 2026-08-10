package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
)

func TestQueueViewKeepsTheNextPromptAndOverflowVisible(t *testing.T) {
	view := newQueueView(kit.Dark(), kit.Unicode())
	view.Set(promptqueue.Snapshot{Entries: []promptqueue.Entry{
		{ID: 1, Message: client.Message{Text: "first follow-up\nwith more detail"}},
		{ID: 2, Message: client.Message{Text: "second follow-up"}},
		{ID: 3, Message: client.Message{Text: "third follow-up"}},
		{ID: 4, Message: client.Message{Text: "fourth follow-up"}},
	}})
	if got := view.Measure(queueMinWidth - 1); got != 0 {
		t.Fatalf("narrow queue height = %d", got)
	}
	if got := view.Measure(80); got != queueMaxRows {
		t.Fatalf("queue height = %d", got)
	}
	rendered := drawStatic(t, view, 48, queueMaxRows)
	for _, want := range []string{"Queue", "4 queued", "first follow-up", "3 more"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("queue does not contain %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "with more detail") {
		t.Fatalf("queue preview leaked later lines:\n%s", rendered)
	}
}

func TestRunningTurnQueuesFollowUpsAndDrainsThemInFIFOOrder(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "PRIMARY_RUN" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: 500 * time.Millisecond, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: client.BlockCompleted{Block: client.Block{ID: "answer-" + prompt, Kind: client.BlockAssistant, Text: "RAN_" + prompt}}},
			{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("PRIMARY_RUN")
	host.Press(input.Enter)
	host.Shows(t, "working")

	host.Type("FOLLOW_UP_ONE")
	host.Press(input.Enter)
	host.Type("FOLLOW_UP_TWO")
	host.Press(input.Enter)
	host.Shows(t, "2 queued")
	host.Shows(t, "FOLLOW_UP_ONE")
	host.Shows(t, "RAN_FOLLOW_UP_TWO")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"PRIMARY_RUN", "FOLLOW_UP_ONE", "FOLLOW_UP_TWO"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestCancelingARunDrainsItsQueuedFollowUpAfterCancellationSettles(t *testing.T) {
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		if prompt == "CANCEL_PRIMARY" {
			return mock.Script{Prelude: []mock.Step{
				{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
			}}
		}
		return mock.Script{Prelude: []mock.Step{
			{Event: client.BlockCompleted{Block: client.Block{ID: "after-cancel", Kind: client.BlockAssistant, Text: "QUEUED_AFTER_CANCEL_RAN"}}},
			{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("CANCEL_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("AFTER_CANCEL")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Press(input.Esc)
	host.Shows(t, "QUEUED_AFTER_CANCEL_RAN")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "AFTER_CANCEL" {
		t.Fatalf("runs after cancellation = %+v", inputs)
	}
	stop()
}

func TestQueuedFollowUpKeepsItsAttachmentIdentityUntilDispatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "context.txt"), []byte("queue context"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := mock.New()
	base.Script = func(prompt string) mock.Script {
		delay := time.Duration(0)
		if prompt == "ATTACHMENT_PRIMARY" {
			delay = 800 * time.Millisecond
		}
		return mock.Script{Prelude: []mock.Step{
			{Delay: delay, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("ATTACHMENT_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("/attach context.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached context.txt")
	host.Type("ATTACHED_FOLLOW_UP")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || len(inputs[1].Message.Attachments) != 1 {
		t.Fatalf("queued attachment inputs = %+v", inputs)
	}
	attachment := inputs[1].Message.Attachments[0]
	wantInfo, wantErr := os.Stat(filepath.Join(workspace, "context.txt"))
	gotInfo, gotErr := os.Stat(attachment.Path)
	if attachment.Name != "context.txt" || wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) {
		t.Fatalf("queued attachment = %+v", attachment)
	}
	stop()
}
