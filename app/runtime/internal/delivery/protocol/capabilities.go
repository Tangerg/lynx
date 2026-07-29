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
	Events           []StreamEventType            `json:"events"`
	StreamingMethods []string                     `json:"streamingMethods"`
	Features         map[string]FeatureCapability `json:"features"`
	Limits           RuntimeLimits                `json:"limits"`
}

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
}
