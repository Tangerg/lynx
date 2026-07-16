package event

import "github.com/Tangerg/lynx/agent/core"

// ModelCallRecorded fires when an LLM call is attributed to a process
// via [core.UsageRecorder.RecordModelCall] or
// [core.UsageRecorder.RecordUsage]. It carries the
// full call so listeners can do per-call cost auditing, billing
// reconciliation, or live token dashboards without polling
// [core.ProcessView.ModelCalls].
type ModelCallRecorded struct {
	Header
	Call core.ModelCall `json:"-"`
}

func (ModelCallRecorded) Kind() string { return "model_call_recorded" }

// EmbeddingCallRecorded mirrors [ModelCallRecorded] for the
// embeddings path.
type EmbeddingCallRecorded struct {
	Header
	Call core.EmbeddingCall `json:"-"`
}

func (EmbeddingCallRecorded) Kind() string { return "embedding_call_recorded" }
