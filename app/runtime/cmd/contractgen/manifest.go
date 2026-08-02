package main

import (
	"cmp"
	"slices"

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
	ValueConstraints []valueConstraintEntry `json:"valueConstraints"`
	SystemInvariants []invariantEntry       `json:"systemInvariants"`
	CanonicalSamples []sampleEntry          `json:"canonicalSamples"`
}

type methodEntry struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Operation   string   `json:"operation"`
	Idempotency string   `json:"idempotency"`
	Pagination  string   `json:"pagination"`
	Errors      []string `json:"errors,omitempty"`
	Features    []string `json:"features,omitempty"`
	Stability   string   `json:"stability"`
}

// errorRegistry is the single source for business error identity (contract §11.4
// gate 13). Types is the symbolic vocabulary clients branch on; each entry carries
// the numeric classification, the default recovery action, whether waiting is
// meaningful, and which methods may return it.
//
// The methods are DERIVED from the registrations rather than declared: every method
// already lists the errors it returns, and a second list is a second answer.
type errorRegistry struct {
	Types    []errorEntry `json:"types"`
	RunTypes []string     `json:"runChannelTypes"`
	Inline   []string     `json:"inlineStatusTypes"`
}

// errorEntry is one published business error.
type errorEntry struct {
	Type              string   `json:"type"`
	Code              int      `json:"code"`
	Recovery          string   `json:"recoveryAction"`
	RetryAfterSeconds int      `json:"retryAfterSeconds,omitempty"`
	Methods           []string `json:"methods,omitempty"`
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
	Type          string `json:"type"`
	Authoritative bool   `json:"authoritative"`
	Replayable    bool   `json:"replayable"`
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
	Type           string             `json:"type"`
	Discriminator  string             `json:"discriminator"`
	Variants       []variantRow       `json:"variants"`
	PatternVariant *patternVariantRow `json:"patternVariant,omitempty"`
	Forbidden      []string           `json:"forbidden,omitempty"`
}

type patternVariantRow struct {
	TagPattern     string   `json:"tagPattern"`
	TypeScriptType string   `json:"typeScriptType"`
	Required       []string `json:"required,omitempty"`
	Optional       []string `json:"optional,omitempty"`
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

type valueConstraintEntry struct {
	Type  string               `json:"type"`
	Rules []valueConstraintRow `json:"rules"`
}

type valueConstraintRow struct {
	Field      string `json:"field"`
	Constraint string `json:"constraint"`
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
		Features:         protocol.FeatureKeys(),
		Methods:          methods(registry),
		Notifications:    notificationNames(shapes),
		StreamingMethods: registry.StreamMethods(),
		Errors:           errors(registry),
		CapabilityPolicy: capabilities(registry),
		RunEventPolicy:   runEvents(shapes),
		RuntimeTopics:    topics(shapes),
		CarriedShapes:    carriedShapes(shapes, walked),
		StatePolicy:      stateKeys(shapes, walked),
		Unions:           unions(shapes),
		Constraints:      constraints(shapes),
		ValueConstraints: valueConstraints(shapes),
		SystemInvariants: invariants(),
		CanonicalSamples: canonicalSamples(),
	}
}

func notificationNames(shapes *dispatch.Shapes) []string {
	notifications := shapes.Notifications()
	out := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		out = append(out, notification.Name)
	}
	return out
}

func methods(registry *dispatch.Registry) []methodEntry {
	metas := registry.Metas()
	out := make([]methodEntry, 0, len(metas))
	for _, meta := range metas {
		out = append(out, methodEntry{
			Name:        meta.Name,
			Kind:        meta.Kind.String(),
			Operation:   meta.Operation.String(),
			Idempotency: meta.Idempotency.String(),
			Pagination:  meta.Pagination.String(),
			Errors:      meta.ProblemTypes(),
			Features:    meta.Features(),
			Stability:   string(meta.Stability),
		})
	}
	return out
}

func errors(registry *dispatch.Registry) errorRegistry {
	// The codes come from the dispatcher's own wire-behavior table: the artifacts
	// must publish the number the runtime actually sends, and a table here was a
	// copy of it.
	codes := dispatch.ProblemCodes()
	return errorRegistry{
		Types: errorTypes(registry, codes),
		// The run/tool channels carry no numeric code — only a symbolic type
		// (API.md §8.4). They are listed so a client's copy table can be checked
		// for completeness against the runtime rather than against a doc.
		RunTypes: dispatch.ProblemTypesFor(dispatch.ProblemChannelExecution),
		// Inline-status problems ride a query's own result instead of failing the
		// call, and deliberately carry no detail — the copy is the client's.
		Inline: dispatch.ProblemTypesFor(dispatch.ProblemChannelInlineStatus),
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
			out = append(out, eventEntry{
				Type: variant.Tag, Authoritative: event.Authoritative(), Replayable: event.Replayable(),
			})
		}
	}
	return out
}

func topics(shapes *dispatch.Shapes) []topicEntry {
	var out []topicEntry
	for _, union := range shapes.Unions() {
		if union.GoType != runtimeEventType {
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
	if topic == string(protocol.RuntimeFilesChanged) {
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
		var pattern *patternVariantRow
		if spec.PatternVariant != nil {
			pattern = &patternVariantRow{
				TagPattern:     spec.PatternVariant.TagPattern,
				TypeScriptType: spec.PatternVariant.TypeScriptType,
				Required:       spec.PatternVariant.Required,
				Optional:       spec.PatternVariant.Optional,
			}
		}
		out = append(out, unionEntry{
			Type: spec.GoType.Name(), Discriminator: spec.Discriminator,
			Variants: variants, PatternVariant: pattern, Forbidden: spec.Forbidden,
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

func valueConstraints(shapes *dispatch.Shapes) []valueConstraintEntry {
	specs := shapes.ValueConstraints()
	out := make([]valueConstraintEntry, 0, len(specs))
	for _, spec := range specs {
		rules := make([]valueConstraintRow, 0, len(spec.Constraints))
		for _, constraint := range spec.Constraints {
			rules = append(rules, valueConstraintRow{
				Field: constraint.Field, Constraint: constraint.String(),
			})
		}
		out = append(out, valueConstraintEntry{Type: spec.GoType.Name(), Rules: rules})
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

// errorTypes publishes one entry per business error: its code, the recovery action
// declared beside its wire behavior, and the methods whose registrations say they
// return it. Sorted by type so the artifact is stable.
func errorTypes(registry *dispatch.Registry, codes map[string]int) []errorEntry {
	byType := make(map[string][]string, len(codes))
	for _, meta := range registry.Metas() {
		for _, problem := range meta.ProblemTypes() {
			byType[problem] = append(byType[problem], meta.Name)
		}
	}
	out := make([]errorEntry, 0, len(codes))
	for problemType, code := range codes {
		recovery, declared := dispatch.RecoveryFor(problemType)
		if !declared {
			panic("contractgen: problem type " + problemType + " declares no recovery action")
		}
		methods := byType[problemType]
		slices.Sort(methods)
		out = append(out, errorEntry{
			Type: problemType, Code: code, Recovery: string(recovery),
			RetryAfterSeconds: dispatch.RetryAfterFor(problemType),
			Methods:           methods,
		})
	}
	slices.SortFunc(out, func(a, b errorEntry) int { return cmp.Compare(a.Type, b.Type) })
	return out
}

// problemCodes is the type→code map, read back from the published registry so the
// OpenRPC document and the manifest cannot state different numbers.
func problemCodes(registry *dispatch.Registry) map[string]int {
	out := make(map[string]int)
	for _, entry := range errors(registry).Types {
		out[entry.Type] = entry.Code
	}
	return out
}
