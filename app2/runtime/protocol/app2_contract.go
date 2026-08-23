package protocol

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// These limits are part of the app2 composition, not knobs. Publishing fixed
// bounds lets a client choose a recovery strategy before it loses a frame.
const (
	DefaultReplayEvents   = 2048
	DefaultReplayBytes    = 16 << 20
	DefaultIdempotencyTTL = 86_400
	DefaultMCPAttemptTTL  = 600
)

// Validator is the small delivery-boundary protocol shared by generated wire
// validation and the lifecycle shapes which are not operation parameters.
type Validator interface {
	Validate() error
}

// InterruptTypes returns a caller-owned snapshot of the complete HITL set.
func InterruptTypes() []InterruptType {
	return []InterruptType{InterruptApproval, InterruptQuestion}
}

// SuppressibleRunEventTypes returns the complete high-frequency preview set.
func SuppressibleRunEventTypes() []SuppressibleRunEventType {
	return []SuppressibleRunEventType{
		SuppressibleRunSegmentProgress,
		SuppressibleRunItemDelta,
	}
}

// RunEventTypes returns the authoritative protocol vocabulary in wire order.
func RunEventTypes() []StreamEventType {
	return []StreamEventType{
		StreamSegmentStarted,
		StreamSegmentProgress,
		StreamSegmentFinished,
		StreamItemStarted,
		StreamItemDelta,
		StreamItemCompleted,
		StreamPlanUpdated,
	}
}

// RunReplayScopes returns the closed replay-scope vocabulary.
func RunReplayScopes() []RunReplayScope {
	return []RunReplayScope{ReplayScopeRuntimeInstanceRootSegment}
}

// Validate checks request metadata that is carried outside operation params.
func (meta RequestMeta) Validate() error {
	if meta.ClientInfo != nil {
		if strings.TrimSpace(meta.ClientInfo.Name) == "" {
			return errors.New("clientInfo.name is required")
		}
		if strings.TrimSpace(meta.ClientInfo.Version) == "" {
			return errors.New("clientInfo.version is required")
		}
	}
	if meta.ClientCapabilities == nil {
		return nil
	}
	if err := validateUniqueClosed(
		"clientCapabilities.interruptTypes",
		meta.ClientCapabilities.InterruptTypes,
		InterruptTypes(),
	); err != nil {
		return err
	}
	return validateUniqueClosed(
		"clientCapabilities.excludedEphemeralEvents",
		meta.ClientCapabilities.ExcludedEphemeralEvents,
		SuppressibleRunEventTypes(),
	)
}

// Validate checks discovery as a coherent promise, including negotiation facts
// and all finite retention limits.
func (response DiscoverResponse) Validate() error {
	switch {
	case response.ProtocolVersion == "":
		return errors.New("protocolVersion is required")
	case response.ServerInfo.InstanceID == "":
		return errors.New("serverInfo.instanceId is required")
	case response.ServerInfo.Name == "":
		return errors.New("serverInfo.name is required")
	case response.ServerInfo.Version == "":
		return errors.New("serverInfo.version is required")
	case response.ServerInfo.DefaultWorkspace.Path == "":
		return errors.New("serverInfo.defaultWorkspace.path is required")
	case response.ServerInfo.Home == "":
		return errors.New("serverInfo.home is required")
	}
	return response.Capabilities.Validate()
}

// Validate rejects a discovery response that advertises a vocabulary or bound
// the same Runtime cannot honor.
func (capabilities ServerCapabilities) Validate() error {
	features := Features()
	if len(capabilities.Features) != len(features) {
		return fmt.Errorf("features has %d entries, want %d", len(capabilities.Features), len(features))
	}
	for _, feature := range features {
		capability, ok := capabilities.Features[feature.Key]
		if !ok {
			return fmt.Errorf("features is missing %q", feature.Key)
		}
		if capability.ClientOptIn != feature.ClientOptIn ||
			capability.RequiredByRunProtocol != feature.RequiredByRunProtocol {
			return fmt.Errorf("features.%s changes published negotiation facts", feature.Key)
		}
	}
	if err := validateUniqueClosed("runEvents", capabilities.RunEvents, RunEventTypes()); err != nil {
		return err
	}
	if err := validateUniqueClosed("runtimeTopics", capabilities.RuntimeTopics, RuntimeTopics()); err != nil {
		return err
	}
	seenMethods := make(map[string]bool, len(capabilities.StreamingMethods))
	for _, method := range capabilities.StreamingMethods {
		parts := strings.Split(method, ".")
		if len(parts) < 2 || slices.Contains(parts, "") {
			return fmt.Errorf("streamingMethods contains invalid method %q", method)
		}
		if seenMethods[method] {
			return fmt.Errorf("streamingMethods contains duplicate %q", method)
		}
		seenMethods[method] = true
	}
	limits := capabilities.Limits
	switch {
	case limits.MaxConcurrentRuns < 0:
		return errors.New("limits.maxConcurrentRuns cannot be negative")
	case limits.Idempotency.RetentionSeconds <= 0:
		return errors.New("limits.idempotency.retentionSeconds must be positive")
	case limits.Idempotency.Namespace == "":
		return errors.New("limits.idempotency.namespace is required")
	case limits.RunReplay.Scope != ReplayScopeRuntimeInstanceRootSegment:
		return errors.New("limits.runReplay.scope is invalid")
	case limits.RunReplay.MaxEvents <= 0:
		return errors.New("limits.runReplay.maxEvents must be positive")
	case limits.RunReplay.MaxBytes <= 0:
		return errors.New("limits.runReplay.maxBytes must be positive")
	case limits.MCPAuthorizationAttempts.RetentionSeconds <= 0:
		return errors.New("limits.mcpAuthorizationAttempts.retentionSeconds must be positive")
	case limits.RuntimeSubscription.MaxTopics <= 0:
		return errors.New("limits.runtimeSubscription.maxTopics must be positive")
	case limits.RuntimeSubscription.MaxWatches <= 0:
		return errors.New("limits.runtimeSubscription.maxWatches must be positive")
	}
	return nil
}

func validateUniqueClosed[T comparable](field string, values, allowed []T) error {
	seen := make(map[T]bool, len(values))
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("%s contains an unknown value", field)
		}
		if seen[value] {
			return fmt.Errorf("%s contains a duplicate", field)
		}
		seen[value] = true
	}
	return nil
}
