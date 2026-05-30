package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// StartRun translates runs.start into the in-process chat.StartTurn
// path. The returned event channel emits AG-UI events (translated
// from Lyra's internal chat.Event stream) until the run ends.
//
// The transport layer wraps each event into a
// notifications/run/event JSON-RPC notification per API.md §3.1.
func (i *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.AgUiEvent, <-chan protocol.RunResult, error) {
	sessionID, err := i.resolveSession(ctx, in.SessionID)
	if err != nil {
		return nil, nil, nil, err
	}

	userMsg := lastUserMessage(in.Messages)
	if userMsg == "" {
		return nil, nil, nil, errors.New("runs.start: messages must end with a user message")
	}

	handle, err := i.rt.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		PlanMode:   in.Mode == protocol.RunModePlan,
		MaxCostUSD: in.MaxBudgetUSD, // API.md §6.3 maxBudgetUsd → turn dollar cap
	})
	if err != nil {
		return nil, nil, nil, err
	}

	inner, err := i.rt.Chat().Events(ctx, handle)
	if err != nil {
		return nil, nil, nil, err
	}

	// runId on the wire == the turn id Lyra assigns internally.
	// API.md v4 §6.3: StartRunResult carries only `runId`. The same
	// id is echoed in every notifications/run/event for client-side
	// filtering — no separate streamHandle.
	runID := handle.TurnID

	out := &protocol.StartRunResponse{RunID: runID}
	events := make(chan protocol.AgUiEvent, 32)
	// Single-shot terminal channel (buffered so pumpRun never blocks on
	// the send): yields one RunResult then closes (API.md §3.1 / §6.3).
	resultCh := make(chan protocol.RunResult, 1)

	runCtx, cancel := context.WithCancel(context.Background())
	i.runMu.Lock()
	i.runs[runID] = &runEntry{
		runID:     runID,
		sessionID: sessionID,
		turnID:    handle.TurnID,
		cancel:    cancel,
	}
	i.runMu.Unlock()

	go i.pumpRun(runCtx, handle, inner, events, resultCh)

	return out, events, resultCh, nil
}

// pumpRun translates internal chat events to AG-UI events and pipes
// them to the consumer, capturing the turn's terminal state along the
// way. Exits when the inner stream closes (turn end) or the run is
// canceled; on exit it sends one RunResult on resultCh (then closes it)
// so the transport can build notifications/run/closed (API.md §6.3).
func (i *Server) pumpRun(ctx context.Context, handle chat.TurnHandle, inner <-chan chat.Event, out chan<- protocol.AgUiEvent, resultCh chan<- protocol.RunResult) {
	translator := agui.NewTranslator(handle.SessionID, handle.TurnID)
	// Defaults to completed in case the stream ends without an explicit
	// TurnEnd; overwritten when TurnEnd / cancellation is observed.
	result := protocol.RunResult{StopReason: "completed"}
	var errMsg string
	defer func() {
		resultCh <- result // buffered cap-1, never blocks
		close(resultCh)
		close(out)
		i.runMu.Lock()
		if e, ok := i.runs[handle.TurnID]; ok {
			e.cancel()
			delete(i.runs, handle.TurnID)
		}
		i.runMu.Unlock()
	}()

	for {
		select {
		case ev, ok := <-inner:
			if !ok {
				return
			}
			switch e := ev.(type) {
			case chat.TurnEnd:
				result = turnEndToRunResult(e)
				if result.StopReason == "error" && errMsg != "" {
					result.Error = &protocol.RunError{Code: runErrorCodeInternal, Message: errMsg}
				}
			case chat.ErrorEvent:
				errMsg = e.Message
			}
			for _, translated := range translator.Translate(ev) {
				select {
				case out <- translated:
				case <-ctx.Done():
					result.StopReason = "canceled"
					return
				}
			}
		case <-ctx.Done():
			// Best-effort cancel of the underlying turn — ignore the
			// error, the turn may have already ended.
			result.StopReason = "canceled"
			_ = i.rt.Chat().Cancel(context.Background(), handle)
			return
		}
	}
}

// runErrorCodeInternal is the placeholder code for a run-execution
// error until chat surfaces a typed code (§7.2). -32603 internal_error.
const runErrorCodeInternal = -32603

// turnEndToRunResult maps the engine's terminal TurnEnd event onto the
// wire RunResult (API.md §6.3). Cost is omitted (nil) when no pricing
// hook produced a figure — never faked to 0.
func turnEndToRunResult(e chat.TurnEnd) protocol.RunResult {
	res := protocol.RunResult{StopReason: stopReasonToWire(e.Reason)}
	u := &protocol.Usage{
		InputTokens:     e.TokenUsage.PromptTokens,
		OutputTokens:    e.TokenUsage.CompletionTokens,
		ReasoningTokens: e.TokenUsage.ReasoningTokens,
	}
	if len(e.UsageByModel) > 0 {
		u.ByModel = make(map[string]protocol.ModelUsage, len(e.UsageByModel))
		for _, m := range e.UsageByModel {
			u.ByModel[m.Model] = protocol.ModelUsage{
				InputTokens:  m.PromptTokens,
				OutputTokens: m.CompletionTokens,
				CostUSD:      optCostUSD(m.CostUSD),
			}
		}
	}
	res.Usage = u
	res.CostUSD = optCostUSD(e.CostUSD)
	return res
}

// optCostUSD returns &c when a pricing hook produced a real figure
// (c > 0), else nil — API.md §6.3 omits cost rather than faking 0.
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}

func stopReasonToWire(r chat.TurnEndReason) string {
	switch r {
	case chat.TurnEndCompleted:
		return "completed"
	case chat.TurnEndCancelled:
		return "canceled"
	case chat.TurnEndErrored:
		return "error"
	case chat.TurnEndBudgetExceeded:
		return "max_budget"
	default:
		return "completed"
	}
}

// CancelRun handles the runs.cancel Request (API.md v4 §3.5 — a
// proper Request method, distinct from notifications/canceled which
// targets in-flight JSON-RPC Requests).
func (i *Server) CancelRun(_ context.Context, runID string) error {
	i.runMu.Lock()
	e, ok := i.runs[runID]
	i.runMu.Unlock()
	if !ok {
		return protocol.ErrRunNotFound
	}
	e.cancel()
	return nil
}

// SubmitApproval handles runs.approval.submit (API.md §4.3). Maps
// the wire decision strings onto the internal enum.
func (i *Server) SubmitApproval(ctx context.Context, in protocol.ApprovalRequest) error {
	if in.RequestID == "" {
		return errors.New("runs.approval.submit: requestId is required")
	}
	dec, err := parseDecision(in.Decision)
	if err != nil {
		return err
	}
	if err := i.rt.Approval().Decide(ctx, in.RequestID, dec); err != nil {
		if errors.Is(err, approval.ErrRequestNotFound) {
			return protocol.ErrRunNotFound
		}
		return err
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────

// resolveSession returns sessionID after verifying it exists, or
// creates a fresh session when sessionID is empty — mirrors the
// auto-create-on-empty path the previous HTTP handler had.
func (i *Server) resolveSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := i.rt.Session().Create(ctx, "")
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := i.rt.Session().Get(ctx, sessionID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", protocol.ErrSessionNotFound
		}
		return "", err
	}
	return sessionID, nil
}

// lastUserMessage extracts the trailing user message text — what the
// in-process chat.StartTurn API expects today. Earlier history is
// already in the session store, so we don't need to thread it.
func lastUserMessage(msgs []protocol.Message) string {
	for idx := len(msgs) - 1; idx >= 0; idx-- {
		if msgs[idx].Role == protocol.MessageRoleUser {
			return msgs[idx].Content
		}
	}
	return ""
}

// parseDecision maps the wire decision string onto the internal enum.
// API.md v4 §4.2: wire values are "approve" | "deny" only — the
// "remember choice" / "always allow" semantic is deliberately not on
// the wire (it's a client-side UI affordance per the protocol
// alignment), so the backend enum is the same two values.
func parseDecision(s string) (approval.Decision, error) {
	switch s {
	case "approve":
		return approval.DecisionApprove, nil
	case "deny":
		return approval.DecisionDeny, nil
	default:
		return 0, errors.New(`decision must be one of "approve" | "deny"`)
	}
}
