package mock

import (
	"slices"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func projectRun(run *runState) client.Run {
	return client.Run{ID: run.id, SessionID: run.sessionID, Status: run.status, StartedAfter: run.startedAfter}
}

func requestKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

func cloneStart(start client.StartRun) client.StartRun {
	start.Message = start.Message.Clone()
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
		out[i] = envelope.Clone()
	}
	return out
}

func cloneEnvelope(envelope client.Envelope) client.Envelope {
	return envelope.Clone()
}

func cloneEvent(event client.Event) client.Event {
	return client.CloneEvent(event)
}
