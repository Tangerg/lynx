package agui

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// AG-UI mock entry point + transport-layer types.
//
// Where everything lives in this package:
//   - server.go              — HTTP listener, /run handler, CORS
//   - run.go                 — Run() dispatcher + RunAgentInput/sender (this file)
//   - streaming.go           — pause + streamText / streamReasoning / streamToolArgs
//   - tools.go               — fireTool (scripted) + telemetry payload helper
//   - dsl.go                 — Step interface + DSL primitives (Say/Think/Pause/...)
//   - demos.go               — threadId → []Step registry + s1 refactor script
//   - demos_zh.go            — Chinese non-coding demo scripts (s2-s7)
//   - reply.go               — runReply (canned follow-up turn)
//   - refactor_demo_data.go  — text constants + tool/plan/search data for the
//                              s1 refactor demo (referenced by demos.go)
//   - events.go              — typed extensions over the community SDK's events
//   - permissions.go         — HITL chan-based approval store
//   - rest.go                — REST endpoints (sessions, projects, terminal, …)
//   - artifacts.go           — static data served via REST
//   - plugins.go             — sideloaded plugin manifest + asset serving

// RunAgentInput — body posted to /run. Only the fields we use are decoded;
// the rest is ignored (passthrough is fine).
type RunAgentInput struct {
	ThreadID string          `json:"threadId"`
	RunID    string          `json:"runId"`
	Messages []ClientMessage `json:"messages"`
}

type ClientMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// EmitFunc reports an event to the HTTP handler. Returns an error if the
// connection has dropped — the runner uses that as a stop signal.
type EmitFunc func(sdkevents.Event) error

// Run dispatches based on whether the agent has any prior turns.
//
// AbstractAgent accumulates messages locally (both user and assistant turns
// added via TEXT_MESSAGE_START events). So we can't use "is there a user
// message?" as the demo trigger — after the demo plays once, every later
// runAgent() call would still carry the demo's user + assistant messages.
//
// Empty messages == fresh agent == play the demo for the active session
// (resolved by threadId). Non-empty == follow-up turn → reply.
func Run(ctx context.Context, input RunAgentInput, emit EmitFunc) {
	if len(input.Messages) == 0 {
		runScript(ctx, input, emit, resolveDemo(input.ThreadID))
		return
	}
	runReply(ctx, input, emit)
}

// sender wraps emit so each call checks ctx + returns a bool. Lets
// scripts and stream helpers fail-fast cleanly when the client
// disconnects, without sprinkling `if err != nil` everywhere.
type sender func(sdkevents.Event) bool

func makeSender(ctx context.Context, emit EmitFunc) sender {
	return func(e sdkevents.Event) bool {
		if ctx.Err() != nil {
			return false
		}
		return emit(e) == nil
	}
}

// newID — 4 random bytes hex-encoded, prefixed with a kind tag.
func newID(prefix string) string {
	var b [4]byte
	_, _ = cryptorand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
