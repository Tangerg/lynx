package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// enforceCapabilities refuses a call whose required features this build did not
// assemble, before any use case runs.
//
// It reads the features from discovery — the same value runtime.discover
// advertises — so "advertised" and "enforced" cannot disagree. That is not a
// convenience: the contract's own drift gate demands the two be equivalent, and
// reading one from the other makes them equivalent by construction rather than by
// a test that has to be remembered.
func (d *Dispatcher) enforceCapabilities(ctx context.Context, meta MethodMeta, params json.RawMessage) *transport.Error {
	if len(meta.CapabilityRules) == 0 {
		return nil
	}
	var frame map[string]any
	for _, rule := range meta.CapabilityRules {
		if len(rule.When) != 0 {
			if frame == nil {
				// Decoded lazily and only for a conditional rule: an unconditional
				// rule never looks at the request, so the common path pays nothing.
				frame = decodeFrame(params)
			}
			if !matchesAll(rule.When, frame) {
				continue
			}
		}
		missing, err := d.missingFeatureRequirements(ctx, rule.Requires)
		if err != nil {
			return errorToRPC(err)
		}
		if len(missing) != 0 {
			return errorToRPC(protocol.NewCapabilityGap(missing...))
		}
	}
	return nil
}

// missingFeatureRequirements returns every feature this runtime/request pair
// cannot use. Discovery answers server support; request metadata answers opt-in.
// A discovery failure is reported rather than swallowed: a gate that cannot read
// the feature set must not decide the call is allowed.
func (d *Dispatcher) missingFeatureRequirements(
	ctx context.Context,
	required []string,
) ([]protocol.CapabilityRequirement, error) {
	discovered, err := d.api.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("dispatch: read capabilities: %w", err)
	}
	if discovered == nil {
		return nil, fmt.Errorf("dispatch: the runtime reported no capabilities")
	}
	var client *protocol.ClientCapabilities
	if declared, ok := protocol.ClientCapabilitiesFrom(ctx); ok {
		client = declared
	}
	return protocol.MissingFeatureRequirements(
		discovered.Capabilities.Features, client, required...,
	), nil
}

// decodeFrame reads the request as a generic JSON frame so a [FieldCondition]
// can address a field by its wire name. Undecodable params yield an empty frame:
// the typed decode has already rejected them, so a condition simply does not
// match and the unconditional rules still apply.
func decodeFrame(params json.RawMessage) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	var frame map[string]any
	if err := json.Unmarshal(params, &frame); err != nil || frame == nil {
		return map[string]any{}
	}
	return frame
}

func matchesAll(conditions []FieldCondition, frame map[string]any) bool {
	for _, condition := range conditions {
		if !condition.matches(frame) {
			return false
		}
	}
	return true
}

func (c FieldCondition) matches(frame map[string]any) bool {
	value, found := lookupField(frame, c.Field)
	switch c.Operator {
	case OperatorPresent:
		return found && !isEmptyJSON(value)
	case OperatorEquals:
		text, ok := value.(string)
		return found && ok && text == c.Value
	default:
		return false
	}
}

// lookupField walks a dotted path through the decoded frame.
func lookupField(frame map[string]any, path string) (any, bool) {
	var value any = frame
	for _, segment := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

// isEmptyJSON treats an explicitly empty value as absent, so a client sending
// `{"watches":[]}` is asking for the same thing as one that omits the field and
// is gated the same way.
func isEmptyJSON(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case bool:
		return !typed
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
