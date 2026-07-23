package sessions

import "context"

// SessionState is the resolved session activity view used by read adapters.
// Running is process-local admission state; Waiting is a durable open HITL
// interrupt; Idle means neither. This precedence is application policy.
type SessionState string

const (
	SessionRunning SessionState = "running"
	SessionWaiting SessionState = "waiting"
	SessionIdle    SessionState = "idle"
)

// SessionStates resolves activity for the requested sessions in one use-case
// read. It centralizes the precedence between a live turn and a durable
// interrupt so Delivery only projects the resolved state.
func (c *Coordinator) SessionStates(ctx context.Context, sessionIDs []string) (map[string]SessionState, error) {
	states := make(map[string]SessionState, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return states, nil
	}
	active := c.admissions.ActiveSessions()
	hasIdle := false
	for _, id := range sessionIDs {
		if active[id] {
			states[id] = SessionRunning
		} else {
			states[id] = SessionIdle
			hasIdle = true
		}
	}
	if !hasIdle || c.interrupts == nil {
		return states, nil
	}
	filter := ""
	if len(sessionIDs) == 1 {
		filter = sessionIDs[0]
	}
	pending, err := c.interrupts.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, interrupt := range pending {
		if states[interrupt.SessionID] == SessionIdle {
			states[interrupt.SessionID] = SessionWaiting
		}
	}
	return states, nil
}
