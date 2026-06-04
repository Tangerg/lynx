package chat

import (
	"context"
	"errors"
	"maps"
	"slices"
)

// ToolLoopStore persists a tool loop suspended mid-round so a later call
// can RESUME the turn — executing only the calls that were still pending
// when it suspended — instead of re-running every completed round (which
// would re-invoke the model and re-execute already-run tools).
//
// It is the seam for human-in-the-loop approval. A tool whose Call
// returns an error implementing ToolLoopSuspend() bool (agent/hitl's
// PauseError) suspends the loop: the middleware captures a
// [ToolLoopCheckpoint] via Save and propagates the suspend error so the
// caller can park the run. When the caller re-enters the middleware for
// the same conversation, Load returns the checkpoint and the loop picks
// up from the suspended round without asking the model again.
//
// Supply one per call via [WithToolLoopStore]; the agent layer backs it
// with the process blackboard so the checkpoint survives the
// suspend/resume round-trip. With NO store on the context the middleware
// behaves exactly as before — the suspend error simply propagates and the
// caller re-runs the whole request.
type ToolLoopStore interface {
	// Load returns a checkpoint to resume from, or (nil, false) to start
	// the loop fresh.
	Load(ctx context.Context) (*ToolLoopCheckpoint, bool)

	// Save persists cp so a later [ToolLoopStore.Load] resumes from it.
	Save(ctx context.Context, cp *ToolLoopCheckpoint) error

	// Clear drops a checkpoint once a resume has consumed it.
	Clear(ctx context.Context) error
}

// ToolLoopCheckpoint is the captured state of a tool loop suspended
// mid-round — enough to resume the round without re-calling the model:
// the conversation the suspended round ran against, that round's
// assistant tool-call message, the tool results already produced this
// round, and the calls still pending (the first of which suspended).
type ToolLoopCheckpoint struct {
	// Messages is the conversation the suspended round's model call ran
	// against — system + history + every completed prior round. It does
	// NOT include Assistant or the round's tool results; those are tracked
	// separately so resume can rebuild the round in order.
	Messages []Message

	// Assistant is the model reply that requested this round's tools — the
	// turn that suspended. Replayed on resume instead of re-asking the
	// model, so completed rounds are never re-billed.
	Assistant *AssistantMessage

	// Done are the tool results already produced in the suspended round
	// (the calls that ran before the suspending one), in call order.
	// Preserved so resume does not re-execute them.
	Done []*ToolReturn

	// Pending are the tool calls not yet executed when the loop suspended,
	// in order; Pending[0] is the call that suspended.
	Pending []*ToolCallPart

	// Iteration is the suspended round's 1-based model-call index, so the
	// resumed loop keeps counting toward the middleware's iteration cap.
	Iteration int
}

// toolLoopStoreCtxKey is the unexported context key carrying a
// [ToolLoopStore] into the tool middleware.
type toolLoopStoreCtxKey struct{}

// WithToolLoopStore returns a ctx carrying store for the tool middleware
// to load/save resume checkpoints against. Thread it through the
// call/stream ctx; a nil store is ignored (returns ctx unchanged).
func WithToolLoopStore(ctx context.Context, store ToolLoopStore) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, toolLoopStoreCtxKey{}, store)
}

// toolLoopStoreFrom returns the [ToolLoopStore] on ctx, or nil.
func toolLoopStoreFrom(ctx context.Context) ToolLoopStore {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(toolLoopStoreCtxKey{}).(ToolLoopStore)
	return store
}

// suspendsToolLoop reports whether a tool error suspends the loop pending
// external input (HITL) rather than aborting or feeding back. A tool
// signals it by returning an error implementing ToolLoopSuspend() bool
// returning true (agent/hitl.PauseError does). Checked before the abort /
// feedback carve-outs in [toolCallInvoker.invokeToolCalls], so suspension
// takes precedence.
func suspendsToolLoop(err error) bool {
	var s interface{ ToolLoopSuspend() bool }
	return errors.As(err, &s) && s.ToolLoopSuspend()
}

// toolRoundSuspension is the partial result of a tool round that
// suspended: the results already produced this invocation (done), the
// calls still pending (the first of which suspended), and the suspend
// error to propagate.
type toolRoundSuspension struct {
	done    []*ToolReturn
	pending []*ToolCallPart
	cause   error
}

// resumeRound runs the pending tool calls of a checkpointed round. On
// success it returns the round's full tool message + its return-direct
// flag. A non-nil error means "propagate to the caller": either another
// gated call in the same round re-suspended (err is the suspend cause,
// already re-checkpointed via store) or a hard failure. Shared by the
// call and stream resume paths.
func (m *ToolMiddleware) resumeRound(ctx context.Context, store ToolLoopStore, support *ToolSupport, cp *ToolLoopCheckpoint) (toolMsg *ToolMessage, allReturnDirect bool, err error) {
	res, err := support.invoker.invokeToolCalls(ctx, cp.Pending)
	if err != nil {
		return nil, false, err
	}

	if res.suspension != nil {
		// Another call in the same round suspended. Fold the results
		// produced so far this resume into the round's done-set and
		// re-checkpoint with the shrunken pending set; the round identity
		// (Messages + Assistant + Iteration) is unchanged. Propagate the
		// suspend cause so the caller parks again.
		merged := append(slices.Clone(cp.Done), res.suspension.done...)
		if store != nil {
			_ = store.Save(ctx, &ToolLoopCheckpoint{
				Messages:  cp.Messages,
				Assistant: cp.Assistant,
				Done:      merged,
				Pending:   res.suspension.pending,
				Iteration: cp.Iteration,
			})
		}
		return nil, false, res.suspension.cause
	}

	returns := append(slices.Clone(cp.Done), res.toolMessage.ToolReturns...)
	toolMsg, err = NewToolMessage(returns)
	if err != nil {
		return nil, false, err
	}
	return toolMsg, allReturnDirectForReturns(support, returns), nil
}

// allReturnDirectForReturns reports whether every tool referenced in
// returns is registered AND return-direct — the resume-path analog of the
// allReturnDirect bit [toolCallInvoker.invokeToolCalls] computes inline.
func allReturnDirectForReturns(support *ToolSupport, returns []*ToolReturn) bool {
	for _, ret := range returns {
		t, exists := support.registry.Find(ret.Name)
		if !exists || !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// resumeCall resumes a checkpointed tool loop on the synchronous path:
// run the pending calls, then either re-suspend, return-direct, or
// continue the loop at the next model round.
func (m *ToolMiddleware) resumeCall(ctx context.Context, cp *ToolLoopCheckpoint, next CallHandler, support *ToolSupport, req *Request) (*Response, error) {
	store := toolLoopStoreFrom(ctx)
	toolMsg, returnDirect, err := m.resumeRound(ctx, store, support, cp)
	if err != nil {
		return nil, err // re-suspension or hard error; store already updated
	}
	if returnDirect {
		return buildResumedReturnResponse(cp.Assistant, toolMsg)
	}
	nextReq, err := buildResumedContinueRequest(req, cp, toolMsg)
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, support, toolLoopState{iteration: cp.Iteration}.next())
}

// resumeStream resumes a checkpointed tool loop on the streaming path. It
// surfaces the resumed round's tool message to the stream (so the wire
// timeline + caller's per-round budget boundary see it) before continuing
// to the next model round.
func (m *ToolMiddleware) resumeStream(ctx context.Context, cp *ToolLoopCheckpoint, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool, req *Request) {
	store := toolLoopStoreFrom(ctx)
	toolMsg, returnDirect, err := m.resumeRound(ctx, store, support, cp)
	if err != nil {
		yield(nil, err) // re-suspension or hard error; store already updated
		return
	}

	if toolMsg != nil {
		if toolResp, e := newToolMessageResponse(toolMsg); e == nil && !yield(toolResp, nil) {
			return
		}
	}
	if returnDirect {
		yield(buildResumedReturnResponse(cp.Assistant, toolMsg))
		return
	}
	nextReq, err := buildResumedContinueRequest(req, cp, toolMsg)
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, support, yield, toolLoopState{iteration: cp.Iteration}.next())
}

// buildResumedContinueRequest assembles the next model request after a
// resumed round completes: the round's conversation + its assistant
// tool-call message + the assembled tool results, carrying the live
// request's options / tools / params.
func buildResumedContinueRequest(req *Request, cp *ToolLoopCheckpoint, toolMsg *ToolMessage) (*Request, error) {
	msgs := append(slices.Clone(cp.Messages), cp.Assistant, toolMsg)
	next, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}

// buildResumedReturnResponse wraps a resumed round's tool message as the
// final response when every tool in the round is return-direct.
func buildResumedReturnResponse(assistant *AssistantMessage, toolMsg *ToolMessage) (*Response, error) {
	result, err := NewResult(assistant, &ResultMetadata{FinishReason: FinishReasonReturnDirect})
	if err != nil {
		return nil, err
	}
	result.ToolMessage = toolMsg
	return NewResponse(result, &ResponseMetadata{})
}

// saveSuspendCheckpoint persists the suspended round to the ctx store (if
// any) and returns the suspend error to propagate. With no store the
// checkpoint is dropped and the caller re-runs the whole request on
// resume (legacy behavior).
func saveSuspendCheckpoint(ctx context.Context, req *Request, resp *Response, susp *toolRoundSuspension, iteration int) error {
	if store := toolLoopStoreFrom(ctx); store != nil {
		_ = store.Save(ctx, &ToolLoopCheckpoint{
			Messages:  req.Messages,
			Assistant: resp.Result.AssistantMessage,
			Done:      susp.done,
			Pending:   susp.pending,
			Iteration: iteration,
		})
	}
	return susp.cause
}
