package main

import (
	appcontract "github.com/Tangerg/lynx/app/runtime/internal/application/contract"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// manifest is the machine-readable projection of the Contract Registry.
//
// Every field here is READ from the registry, never restated. That is the whole
// value: a client, a schema, or a reviewer reads this file instead of grepping
// the dispatcher, and `go generate` leaving a diff means the artifacts drifted
// from the code — which is the only way to notice.
type manifest struct {
	Protocol         protocol.ProtocolRange `json:"protocol"`
	Features         []string               `json:"features"`
	Methods          []methodEntry          `json:"methods"`
	Notifications    []string               `json:"notifications"`
	StreamingMethods []string               `json:"streamingMethods"`
	Errors           errorRegistry          `json:"errors"`
	CapabilityPolicy []capabilityEntry      `json:"capabilityPolicy"`
	RunEventPolicy   []eventEntry           `json:"runEventPolicy"`
	RuntimeTopics    []topicEntry           `json:"runtimeTopics"`
	CarriedShapes    []carriedEntry         `json:"carriedShapes"`
	StatePolicy      []stateEntry           `json:"statePolicy"`
	Unions           []unionEntry           `json:"unions"`
	Constraints      []constraintEntry      `json:"objectConstraints"`
	SystemInvariants []invariantEntry       `json:"systemInvariants"`
}

type methodEntry struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Idempotency string   `json:"idempotency"`
	Errors      []string `json:"errors,omitempty"`
	Features    []string `json:"features,omitempty"`
	Stability   string   `json:"stability"`
}

// errorRegistry is the single source for business error identity (contract
// §11.4 gate 13). Codes are the numeric classification; Types is the symbolic
// vocabulary clients actually branch on.
type errorRegistry struct {
	Codes    map[string]int `json:"codes"`
	RunTypes []string       `json:"runChannelTypes"`
	Inline   []string       `json:"inlineStatusTypes"`
}

type capabilityEntry struct {
	Method string          `json:"method"`
	Rules  []capabilityRow `json:"rules"`
}

type capabilityRow struct {
	When     []conditionRow `json:"when,omitempty"`
	Requires []string       `json:"requires"`
}

type conditionRow struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

// eventEntry is the reliability classification API.md §5.2 derives from the
// event type. It is emitted so a client can build its replay/dedup logic from
// data instead of re-implementing the derivation table.
type eventEntry struct {
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	Replayable bool   `json:"replayable"`
}

type topicEntry struct {
	Type    string `json:"type"`
	Feature string `json:"feature,omitempty"`
}

type stateEntry struct {
	Key            string  `json:"key"`
	RecoveryMethod string  `json:"recoveryMethod"`
	Scope          string  `json:"scope"`
	Writer         string  `json:"writer"`
	Feature        string  `json:"feature"`
	Stability      string  `json:"stability"`
	Payload        *schema `json:"payload"`
}

// carriedEntry says where on the wire a shape rides that no method frame reaches.
// The carrier is part of the answer: a bare type list would publish the shape
// without saying where a client would ever meet it.
type carriedEntry struct {
	Carrier string  `json:"carrier"`
	Schema  *schema `json:"schema"`
}

type unionEntry struct {
	Type          string       `json:"type"`
	Discriminator string       `json:"discriminator"`
	Variants      []variantRow `json:"variants"`
}

type variantRow struct {
	Tag      string   `json:"tag"`
	Required []string `json:"required,omitempty"`
	Optional []string `json:"optional,omitempty"`
}

type constraintEntry struct {
	Type  string          `json:"type"`
	Rules []constraintRow `json:"rules"`
}

type constraintRow struct {
	When      []conditionRow `json:"when,omitempty"`
	Required  []string       `json:"required,omitempty"`
	Forbidden []string       `json:"forbidden,omitempty"`
}

type invariantEntry struct {
	Key        string   `json:"key"`
	Why        string   `json:"why"`
	Boundaries []string `json:"boundaries"`
}

func build(walked *schemaSet) manifest {
	registry := dispatch.Contract()
	shapes := dispatch.WireShapes()
	return manifest{
		Protocol:         protocol.SupportedProtocolRange(),
		Features:         protocol.Features,
		Methods:          methods(registry),
		Notifications:    []string{dispatch.NotificationRunEvent, dispatch.NotificationWorkspaceEvent},
		StreamingMethods: registry.StreamMethods(),
		Errors:           errors(),
		CapabilityPolicy: capabilities(registry),
		RunEventPolicy:   runEvents(shapes),
		RuntimeTopics:    topics(shapes),
		CarriedShapes:    carriedShapes(shapes, walked),
		StatePolicy:      stateKeys(shapes, walked),
		Unions:           unions(shapes),
		Constraints:      constraints(shapes),
		SystemInvariants: invariants(),
	}
}

func methods(registry *dispatch.Registry) []methodEntry {
	metas := registry.Metas()
	out := make([]methodEntry, 0, len(metas))
	for _, meta := range metas {
		out = append(out, methodEntry{
			Name:        meta.Name,
			Kind:        meta.Kind.String(),
			Idempotency: meta.Idempotency.String(),
			Errors:      meta.Errors,
			Features:    meta.Features(),
			Stability:   string(meta.Stability),
		})
	}
	return out
}

func errors() errorRegistry {
	return errorRegistry{
		Codes: map[string]int{
			protocol.ErrMethodNotFound.Error():         protocol.CodeMethodNotFound,
			protocol.ErrInvalidParams.Error():          protocol.CodeInvalidParams,
			protocol.ErrProviderError.Error():          protocol.CodeProviderError,
			protocol.ErrSessionNotFound.Error():        protocol.CodeSessionNotFound,
			protocol.ErrRunNotFound.Error():            protocol.CodeRunNotFound,
			protocol.ErrItemNotFound.Error():           protocol.CodeItemNotFound,
			protocol.ErrCwdUnavailable.Error():         protocol.CodeCwdUnavailable,
			protocol.ErrCapabilityNotNeg.Error():       protocol.CodeCapabilityNotNeg,
			protocol.ErrRunAlreadyDone.Error():         protocol.CodeRunAlreadyDone,
			protocol.ErrCheckpointUnavailable.Error():  protocol.CodeCheckpointUnavail,
			protocol.ErrUnsupportedMime.Error():        protocol.CodeUnsupportedMime,
			protocol.ErrPathOutsideRoot.Error():        protocol.CodePathOutsideRoot,
			protocol.ErrInterruptNotOpen.Error():       protocol.CodeInterruptNotOpen,
			protocol.ErrInvalidProtocolVersion.Error(): protocol.CodeInvalidProtocolVersion,
			protocol.ErrVcsUnavailable.Error():         protocol.CodeVcsUnavailable,
			protocol.ErrSessionBusy.Error():            protocol.CodeSessionBusy,
			protocol.ErrRevisionConflict.Error():       protocol.CodeRevisionConflict,
			protocol.ErrIdempotencyConflict.Error():    protocol.CodeIdempotencyConflict,
			protocol.ErrIdempotencyInProgress.Error():  protocol.CodeIdempotencyInProgress,
		},
		// The run/tool channels carry no numeric code — only a symbolic type
		// (API.md §8.4). They are listed so a client's copy table can be checked
		// for completeness against the runtime rather than against a doc.
		RunTypes: []string{
			protocol.ProblemInternalError,
			protocol.ProblemRunLost,
			protocol.ProblemAgentStuck,
			protocol.ProblemRateLimited,
			protocol.ProblemInvalidAPIKey,
			protocol.ProblemTimeout,
			protocol.ProblemProviderUnavailable,
			protocol.ProblemProviderRejected,
			protocol.ProblemDeniedByUser,
			protocol.ProblemToolFailed,
		},
		// Inline-status problems ride a query's own result instead of failing the
		// call, and deliberately carry no detail — the copy is the client's.
		Inline: []string{
			protocol.ProblemMCPAuthorizationRequired,
			protocol.ProblemMCPDialFailed,
			protocol.ProblemProviderNotConfigured,
			protocol.ProblemProviderTestFailed,
		},
	}
}

func capabilities(registry *dispatch.Registry) []capabilityEntry {
	var out []capabilityEntry
	for _, meta := range registry.Metas() {
		if len(meta.CapabilityRules) == 0 {
			continue
		}
		rows := make([]capabilityRow, 0, len(meta.CapabilityRules))
		for _, rule := range meta.CapabilityRules {
			rows = append(rows, capabilityRow{When: conditions(rule.When), Requires: rule.Requires})
		}
		out = append(out, capabilityEntry{Method: meta.Name, Rules: rows})
	}
	return out
}

func conditions(in []dispatch.FieldCondition) []conditionRow {
	if len(in) == 0 {
		return nil
	}
	out := make([]conditionRow, 0, len(in))
	for _, condition := range in {
		out = append(out, conditionRow{
			Field: condition.Field, Operator: condition.Operator.String(), Value: condition.Value,
		})
	}
	return out
}

// runEvents emits the §5.2 derivation table by ASKING the code that decides it,
// so the artifact cannot disagree with the hub's replay buffer or the SSE id gate.
func runEvents(shapes *dispatch.Shapes) []eventEntry {
	var out []eventEntry
	for _, union := range shapes.Unions() {
		if union.GoType != streamEventType {
			continue
		}
		for _, variant := range union.Variants {
			event := protocol.StreamEvent{Type: protocol.StreamEventType(variant.Tag)}
			durable := event.IsDurable()
			out = append(out, eventEntry{Type: variant.Tag, Durable: durable, Replayable: durable})
		}
	}
	return out
}

func topics(shapes *dispatch.Shapes) []topicEntry {
	var out []topicEntry
	for _, union := range shapes.Unions() {
		if union.GoType != workspaceEventType {
			continue
		}
		for _, variant := range union.Variants {
			out = append(out, topicEntry{Type: variant.Tag, Feature: topicFeature(variant.Tag)})
		}
	}
	return out
}

// topicFeature reports the feature a topic's PRODUCTION depends on. files.changed
// only exists for a client that registered watches; the rest are unconditional
// (AUX_API §1).
func topicFeature(topic string) string {
	if topic == string(protocol.WorkspaceEventFilesChanged) {
		return protocol.FeatureFileWatch
	}
	return ""
}

func stateKeys(shapes *dispatch.Shapes, walked *schemaSet) []stateEntry {
	keys := shapes.StateKeys()
	out := make([]stateEntry, 0, len(keys))
	for _, key := range keys {
		out = append(out, stateEntry{
			Key: key.Key, RecoveryMethod: key.RecoveryMethod,
			Scope: string(key.Scope), Writer: string(key.Writer),
			Feature: key.Feature, Stability: string(key.Stability),
			Payload: external(walked.walk(key.PayloadType)),
		})
	}
	return out
}

func carriedShapes(shapes *dispatch.Shapes, walked *schemaSet) []carriedEntry {
	carried := shapes.Carried()
	out := make([]carriedEntry, 0, len(carried))
	for _, shape := range carried {
		out = append(out, carriedEntry{Carrier: shape.Carrier, Schema: external(walked.walk(shape.GoType))})
	}
	return out
}

func unions(shapes *dispatch.Shapes) []unionEntry {
	specs := shapes.Unions()
	out := make([]unionEntry, 0, len(specs))
	for _, spec := range specs {
		variants := make([]variantRow, 0, len(spec.Variants))
		for _, variant := range spec.Variants {
			variants = append(variants, variantRow{
				Tag: variant.Tag, Required: variant.Required, Optional: variant.Optional,
			})
		}
		out = append(out, unionEntry{
			Type: spec.GoType.Name(), Discriminator: spec.Discriminator, Variants: variants,
		})
	}
	return out
}

func constraints(shapes *dispatch.Shapes) []constraintEntry {
	specs := shapes.Constraints()
	out := make([]constraintEntry, 0, len(specs))
	for _, spec := range specs {
		rules := make([]constraintRow, 0, len(spec.Rules))
		for _, rule := range spec.Rules {
			rules = append(rules, constraintRow{
				When: conditions(rule.When), Required: rule.Required, Forbidden: rule.Forbidden,
			})
		}
		out = append(out, constraintEntry{Type: spec.GoType.Name(), Rules: rules})
	}
	return out
}

func invariants() []invariantEntry {
	specs := appcontract.SystemInvariants()
	out := make([]invariantEntry, 0, len(specs))
	for _, spec := range specs {
		boundaries := make([]string, 0, len(spec.Boundaries))
		for _, boundary := range spec.Boundaries {
			boundaries = append(boundaries, string(boundary))
		}
		out = append(out, invariantEntry{Key: spec.Key, Why: spec.Why, Boundaries: boundaries})
	}
	return out
}
