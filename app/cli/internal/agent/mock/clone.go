package mock

import (
	"slices"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectRun(run *runState) agent.Run {
	return agent.Run{ID: run.id, SessionID: run.sessionID, Status: run.status, StartedAfter: run.startedAfter}
}

func requestKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

func cloneStart(start agent.StartRun) agent.StartRun {
	start.Message = start.Message.Clone()
	return start
}

func sameStart(a, b agent.StartRun) bool {
	return a.RequestID == b.RequestID &&
		a.SessionID == b.SessionID &&
		a.Message.Text == b.Message.Text &&
		a.Options == b.Options &&
		slices.Equal(a.Message.Attachments, b.Message.Attachments)
}

func cloneEnvelopes(events []agent.Envelope) []agent.Envelope {
	out := make([]agent.Envelope, len(events))
	for i, envelope := range events {
		out[i] = envelope.Clone()
	}
	return out
}

func cloneEnvelope(envelope agent.Envelope) agent.Envelope {
	return envelope.Clone()
}

func cloneEvent(event agent.Event) agent.Event {
	return agent.CloneEvent(event)
}
