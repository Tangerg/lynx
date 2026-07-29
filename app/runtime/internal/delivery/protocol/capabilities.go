package protocol

import (
	"errors"
	"fmt"
)

// ClientCapabilities is what the client declares in request metadata (§8.1).
//
// It declares what the client can HANDLE, not what it wants to receive. There is
// deliberately no list of renderable event types: a client that cannot follow the
// authoritative stream cannot be served a shortened one — the §8.3 Minimal
// Profile requires it to validate and safely fold or ignore every authoritative
// frame — so filtering by declaration would hand it a degraded stream while
// letting it believe the stream was complete.
type ClientCapabilities struct {
	Features map[string]FeaturePreference `json:"features,omitempty"`
	// InterruptTypes are the HITL interrupt types the client can answer. A run
	// freezes them into its RunProtocolProfile, so the runtime can never park on a
	// wait nobody will answer (§6.2 anti-deadlock). Omitted and empty mean the
	// same thing: a client that answers nothing.
	InterruptTypes []InterruptType `json:"interruptTypes,omitempty"`
	// ExcludedEphemeralEvents lets a client suppress high-frequency previews per
	// request, e.g. [StreamItemDelta]. Only an always-ephemeral type may appear:
	// dropping a durable event would break the §5.2 guarantee that a client which
	// discards every ephemeral event still converges, so an authoritative type here
	// is refused rather than ignored. It does not reach the workspace stream —
	// that stream's scoping is its subscription, not an exclusion list.
	ExcludedEphemeralEvents []StreamEventType `json:"excludedEphemeralEvents,omitempty"`
}

// ErrNotEphemeral reports an exclusion list naming an event the client may not
// opt out of.
var ErrNotEphemeral = errors.New("excludedEphemeralEvents may only name an ephemeral event type")

// Validate refuses a declaration the runtime cannot honor. It is checked where
// request metadata is decoded, so an unhonorable exclusion is an invalid request
// rather than a preference the runtime quietly overrules.
func (c ClientCapabilities) Validate() error {
	for _, event := range c.ExcludedEphemeralEvents {
		if !event.AlwaysEphemeral() {
			return fmt.Errorf("%w: %q", ErrNotEphemeral, event)
		}
	}
	return nil
}

type FeaturePreference struct {
	Enabled bool `json:"enabled"`
}

// ServerCapabilities is what Runtime advertises in runtime.discover
// and the /v2/info sidecar (API.md §9).
type ServerCapabilities struct {
	// RunEvents is the stream-event vocabulary a run publishes. It was called
	// `events`, which said nothing about WHICH stream — and there are two.
	RunEvents []StreamEventType `json:"runEvents"`
	// RuntimeTopics is what runtime.subscribe accepts. Advertised from the same
	// registry the subscribe request is validated against, so a client cannot be
	// refused for asking exactly what discovery offered.
	RuntimeTopics []RuntimeTopic `json:"runtimeTopics"`
	// StateSnapshots are the durable projections THIS composition both writes and
	// can serve a cold read for. A client builds a projection only for an advertised
	// key, and each entry carries the registry's own scope and writer: an SDK reads
	// them to pick its reducer identity, instead of assuming every state belongs to
	// the current run.
	StateSnapshots   []StateSnapshotCapability    `json:"stateSnapshots"`
	StreamingMethods []string                     `json:"streamingMethods"`
	Features         map[string]FeatureCapability `json:"features"`
	Limits           RuntimeLimits                `json:"limits"`
}

// StateSnapshotCapability is one advertised state key with the facts an SDK needs to
// fold and to recover it (§5.6). It is a registry descriptor, not a wire union: the
// key is a name, not a discriminator.
type StateSnapshotCapability struct {
	Key StateSnapshotType `json:"key"`
	// RecoveryMethod is the cold read for this key. A key with no read is not
	// advertised, because "recover it" would have no answer.
	RecoveryMethod string `json:"recoveryMethod"`
	// Scope decides the projection's identity — which value a client is holding —
	// and Writer decides which run may change it.
	Scope  StateSnapshotScope  `json:"scope"`
	Writer StateSnapshotWriter `json:"writer"`
}

// StateSnapshotScope is what a state key is keyed BY. A session-scoped key is one
// value per session; a run-scoped one is one per run, and its payload and recovery
// request must both carry the runId.
type StateSnapshotScope string

const (
	StateScopeSession StateSnapshotScope = "session"
	StateScopeRun     StateSnapshotScope = "run"
)

// StateSnapshotWriter is which run in a tree may change a key.
type StateSnapshotWriter string

const (
	StateWriterRootRun StateSnapshotWriter = "rootRun"
	StateWriterAnyRun  StateSnapshotWriter = "anyRun"
)

type Stability string

const (
	StabilityStable       Stability = "stable"
	StabilityExperimental Stability = "experimental"
)

// FeatureCapability is one advertised capability: whether this build offers it,
// and the two negotiation facts that belong to the feature itself rather than to
// the build ([Feature]).
type FeatureCapability struct {
	Enabled   bool      `json:"enabled"`
	Stability Stability `json:"stability"`
	// ClientOptIn says server support is not sufficient: the runtime uses the
	// feature only for a request whose capabilities declare it. A client that reads
	// `enabled:true` and never declares it still gets the feature's absence.
	ClientOptIn bool `json:"clientOptIn"`
	// RequiredByRunProtocol says negotiating it changes the authoritative Run
	// event / resource shape, so a subscriber that does not understand it cannot
	// follow the Run. Exactly these keys enter [RunProtocolProfile.RequiredFeatures]
	// and make a later low-capability resume or subscribe a refusal rather than a
	// downgrade (§8.2).
	RequiredByRunProtocol bool `json:"requiredByRunProtocol"`
}

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns int `json:"maxConcurrentRuns,omitempty"`
	// RunReplay is what a reconnecting subscriber can expect to get back. Published
	// because the alternative is a client discovering the ceiling by losing events:
	// knowing the bound is what lets it choose replay or a cold read.
	//
	// Absent until the bound exists. The journal currently retains a segment's whole
	// durable history, so there is no ceiling to state — and stating a number the
	// runtime does not enforce would be discovery lying, which is the one thing
	// discovery may never do. C13 introduces the retention window and the refusal a
	// cursor past it gets, and fills this in.
	RunReplay *RunReplayLimits `json:"runReplay,omitempty"`
	// RuntimeSubscription bounds one subscription's fan-out.
	RuntimeSubscription SubscriptionLimits `json:"runtimeSubscription"`
}

// RunReplayScope is what a replay buffer belongs to. One value today, and it is named
// rather than implied: a client that assumed the buffer spanned a whole run would
// expect replay across a restart, and across segments, from a buffer that holds
// neither.
type RunReplayScope string

const (
	// ReplayScopeProcessRootSegment — this process, this root segment. A restart or a
	// new segment starts a new buffer.
	ReplayScopeProcessRootSegment RunReplayScope = "processRootSegment"
)

// RunReplayLimits is the retention a reconnect can count on.
type RunReplayLimits struct {
	Scope     RunReplayScope `json:"scope"`
	MaxEvents int            `json:"maxEvents"`
	MaxBytes  int            `json:"maxBytes"`
}

// SubscriptionLimits caps one runtime subscription.
type SubscriptionLimits struct {
	MaxTopics  int `json:"maxTopics"`
	MaxWatches int `json:"maxWatches"`
}
