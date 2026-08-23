package protocol

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
	// ExcludedEphemeralEvents lets a client suppress the two high-frequency
	// previews. Its dedicated closed type makes an authoritative event
	// unrepresentable instead of accepting the broad StreamEventType and checking
	// it later. It does not reach the workspace stream.
	ExcludedEphemeralEvents []SuppressibleRunEventType `json:"excludedEphemeralEvents,omitempty"`
}

// SuppressibleRunEventType is the complete client-suppressible run-event set.
type SuppressibleRunEventType string

const (
	SuppressibleRunSegmentProgress SuppressibleRunEventType = "segment.progress"
	SuppressibleRunItemDelta       SuppressibleRunEventType = "item.delta"
)

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
	RuntimeTopics    []RuntimeTopic               `json:"runtimeTopics"`
	StreamingMethods []string                     `json:"streamingMethods"`
	Features         map[string]FeatureCapability `json:"features"`
	Limits           RuntimeLimits                `json:"limits"`
}

// FeatureCapability is one advertised capability: whether this build offers it,
// and the two negotiation facts that belong to the feature itself rather than to
// the build ([Feature]).
type FeatureCapability struct {
	Enabled bool `json:"enabled"`
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

// MissingFeatureRequirements evaluates one request against discovery's feature
// facts. A feature is usable only when this runtime enables it and, for an opt-in
// feature, this request explicitly enables it too.
//
// Both the Registry's shape-dependent gate and state-dependent server gates call
// this function. Keeping the decision here prevents "server supports it" and
// "client negotiated it" from becoming two subtly different policies.
func MissingFeatureRequirements(
	advertised map[string]FeatureCapability,
	client *ClientCapabilities,
	required ...string,
) []CapabilityRequirement {
	seen := make(map[string]bool, len(required))
	var missing []CapabilityRequirement
	for _, key := range required {
		if seen[key] {
			continue
		}
		seen[key] = true

		server := advertised[key]
		available := server.Enabled
		if available && server.ClientOptIn {
			available = client != nil && client.Features[key].Enabled
		}
		if !available {
			missing = append(missing, CapabilityRequirement{
				Type: RequirementFeature, Name: key,
			})
		}
	}
	return missing
}

// RuntimeLimits — server-side hard caps surfaced to the client.
type RuntimeLimits struct {
	MaxConcurrentRuns int `json:"maxConcurrentRuns,omitempty"`
	// Idempotency tells clients how long a command's first response remains
	// replayable under the same Idempotency-Key. A client must not invent this
	// window: retrying after it expires may execute the command again.
	Idempotency IdempotencyLimits `json:"idempotency"`
	// RunReplay is what a reconnecting subscriber can expect to get back. Published
	// because the alternative is a client discovering the ceiling by losing events:
	// knowing the bound is what lets it choose replay or a cold read.
	//
	// This is a required value rather than an optional promise because every
	// runtime subscription enforces a finite replay window.
	RunReplay RunReplayLimits `json:"runReplay"`
	// MCPAuthorizationAttempts tells clients how long terminal OAuth outcomes
	// remain queryable. Pending attempts are retained until they settle.
	MCPAuthorizationAttempts MCPAuthorizationAttemptLimits `json:"mcpAuthorizationAttempts"`
	// RuntimeSubscription bounds one subscription's fan-out.
	RuntimeSubscription SubscriptionLimits `json:"runtimeSubscription"`
}

// IdempotencyLimits is the replay promise for completed command results.
// Namespace identifies the exact durable replay store without exposing a path;
// clients must not restore a persisted key under a different namespace. Pending
// reservations remain bound until an outcome is known.
type IdempotencyLimits struct {
	RetentionSeconds int    `json:"retentionSeconds"`
	Namespace        string `json:"namespace"`
}

// MCPAuthorizationAttemptLimits is the retention promise for terminal
// interactive authorization resources.
type MCPAuthorizationAttemptLimits struct {
	RetentionSeconds int `json:"retentionSeconds"`
}

// RunReplayScope is what a replay buffer belongs to. One value today, and it is named
// rather than implied: a client that assumed the buffer spanned a whole run would
// expect replay across a restart, and across segments, from a buffer that holds
// neither.
type RunReplayScope string

const (
	// ReplayScopeRuntimeInstanceRootSegment — this Runtime instance and this root
	// segment. A Runtime restart or a new segment starts a new buffer.
	ReplayScopeRuntimeInstanceRootSegment RunReplayScope = "runtimeInstanceRootSegment"
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
