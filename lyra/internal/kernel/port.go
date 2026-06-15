package kernel

import "context"

// This file is the microkernel's port surface: the narrow interfaces the engine
// core consumes, defined here (the consumer) per DIP. Implementations live in
// domain/* and are injected by the composition root (runtime) via [Config];
// the engine core imports no concrete service. Every port use is nil-guarded,
// so an engine built without a given port simply no-ops that capability (used
// by unit tests that drive only the loop). See doc/GREENFIELD_ARCHITECTURE.md §5.1.

// CompactionResult reports what a single [Compactor.MaybeCompact] sweep did.
// Compacted is false (counts zero) when the sweep didn't fire — no session,
// history below threshold, or no compactor wired. The before/after counts let
// the caller surface an observable "context compacted (N → M)" boundary.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// ExtractionResult reports what a single [Extractor.MaybeExtract] pass wrote to
// long-term memory. Extracted is false (Facts empty) when nothing was mined.
// Facts is the markdown appended to LYRA.md, for a "saved N notes" event.
type ExtractionResult struct {
	Extracted bool
	Facts     string
}

// SteeringSink is the engine's turn-lifecycle seam for mid-turn steering: chat
// flushes a queued steering message through here at turn-end so the next turn
// sees it as part of the conversation. It is the ONLY message-history operation
// the engine core touches — steering is a turn concern. The rest of history
// management (read / seed / count / truncate, for fork / rollback /
// messages.list) is NOT a turn concern and is driven directly off
// domain/conversation by the runtime, never proxied through the engine.
// Implemented by domain/conversation.
type SteeringSink interface {
	InjectUser(ctx context.Context, sessionID, text string) error
}

// Compactor folds an over-long history into a summary at a turn boundary.
// Implemented by domain/maintenance.
type Compactor interface {
	MaybeCompact(ctx context.Context, sessionID string) (CompactionResult, error)
}

// Extractor mines the recent conversation for facts worth keeping in the
// project's LYRA.md. Implemented by domain/maintenance.
type Extractor interface {
	MaybeExtract(ctx context.Context, sessionID, cwd string) (ExtractionResult, error)
}
