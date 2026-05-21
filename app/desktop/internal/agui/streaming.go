package agui

import (
	"context"
	"math/rand/v2"
	"time"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// Streaming primitives — shared by the DSL runner (dsl.go) and the
// canned reply path (reply.go).
//
// Each primitive:
//   - takes a context for cancellation (client disconnect)
//   - takes a sender (see run.go's makeSender) so the call site never
//     re-checks errors / ctx inline
//   - returns false to bubble cancellation back to the caller
//
// Tunables live as constants at the top of the file. Raising the chunk
// size makes streaming feel chunkier, lowering it feels finer. The
// per-chunk pause range adds jitter so the cadence reads as human/LLM
// rather than mechanical.

// pause sleeps for a uniformly-random duration in [minMs, maxMs] ms.
// Returns false if ctx is canceled mid-wait. Use it between major beats
// (before a tool call, after a reasoning span ends, before an approval
// pops) so a script doesn't fire every event back-to-back.
func pause(ctx context.Context, minMs, maxMs int) bool {
	if maxMs < minMs {
		minMs, maxMs = maxMs, minMs
	}
	d := time.Duration(minMs+rand.IntN(maxMs-minMs+1)) * time.Millisecond
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// streamText emits `text` as token-sized chunks (4-9 runes) with jittered
// cadence (40-100ms per chunk), targeting ~110 chars/sec. Roughly half
// the speed of a fast LLM stream so the fade-in animation has time to
// register per word.
func streamText(ctx context.Context, send sender, messageID, text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); {
		n := 4 + rand.IntN(6) // 4..9
		end := min(i+n, len(runes))
		if !send(sdkevents.NewTextMessageContentEvent(messageID, string(runes[i:end]))) {
			return false
		}
		i = end
		if !pauseFor(ctx, 40+rand.IntN(61)) {
			return false
		}
	}
	return true
}

// streamReasoning streams a reasoning body with larger chunks (10-18
// runes) at 50-110ms — reasoning is meant to feel deliberative, not
// transcribed, so even slower than text streaming.
func streamReasoning(ctx context.Context, send sender, messageID, text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); {
		n := 10 + rand.IntN(9) // 10..18
		end := min(i+n, len(runes))
		if !send(sdkevents.NewReasoningMessageContentEvent(messageID, string(runes[i:end]))) {
			return false
		}
		i = end
		if !pauseFor(ctx, 50+rand.IntN(61)) {
			return false
		}
	}
	return true
}

// streamToolArgs splits the argument string into 3-7 rune chunks with
// 30-75ms gaps. Tool args render in a small inline card so they don't
// need to feel as fluid as the main message stream.
func streamToolArgs(ctx context.Context, send sender, toolID, args string) bool {
	runes := []rune(args)
	if len(runes) == 0 {
		return true
	}
	for i := 0; i < len(runes); {
		n := 3 + rand.IntN(5) // 3..7
		end := min(i+n, len(runes))
		if !send(sdkevents.NewToolCallArgsEvent(toolID, string(runes[i:end]))) {
			return false
		}
		i = end
		if !pauseFor(ctx, 30+rand.IntN(46)) {
			return false
		}
	}
	return true
}

// pauseFor sleeps a fixed millisecond duration with ctx cancellation.
// Used by stream loops where the random range is already computed.
func pauseFor(ctx context.Context, ms int) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return true
	}
}
