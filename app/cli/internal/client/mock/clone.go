package mock

import (
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func projectRun(run *runState) client.Run {
	return client.Run{ID: run.id, SessionID: run.sessionID, Status: run.status, StartedAfter: run.startedAfter}
}

func requestKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

func cloneStart(start client.StartRun) client.StartRun {
	start.Message.Text = strings.Clone(start.Message.Text)
	start.Message.Attachments = slices.Clone(start.Message.Attachments)
	return start
}

func sameStart(a, b client.StartRun) bool {
	return a.RequestID == b.RequestID &&
		a.SessionID == b.SessionID &&
		a.Message.Text == b.Message.Text &&
		a.Options == b.Options &&
		slices.Equal(a.Message.Attachments, b.Message.Attachments)
}

func cloneEnvelopes(events []client.Envelope) []client.Envelope {
	out := make([]client.Envelope, len(events))
	for i, envelope := range events {
		out[i] = cloneEnvelope(envelope)
	}
	return out
}

func cloneEnvelope(envelope client.Envelope) client.Envelope {
	envelope.Event = cloneEvent(envelope.Event)
	return envelope
}

func cloneEvent(event client.Event) client.Event {
	switch item := event.(type) {
	case client.RunStarted:
		return item
	case client.RunResumed:
		return item
	case client.BlockStarted:
		item.Block = cloneBlock(item.Block)
		return item
	case client.BlockDelta:
		return item
	case client.BlockCompleted:
		item.Block = cloneBlock(item.Block)
		return item
	case client.PlanChanged:
		item.Items = slices.Clone(item.Items)
		return item
	case client.RunInterrupted:
		item.Interaction = client.CloneInteraction(item.Interaction)
		return item
	case client.RunFinished:
		return item
	default:
		return nil
	}
}

func cloneBlock(block client.Block) client.Block {
	block.Attachments = slices.Clone(block.Attachments)
	if block.Tool != nil {
		tool := *block.Tool
		if block.Tool.ExitCode != nil {
			code := *block.Tool.ExitCode
			tool.ExitCode = &code
		}
		block.Tool = &tool
	}
	return block
}
