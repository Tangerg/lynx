package arch

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Contract §11.4 gates 16 and 17: the compatibility differ.
//
// The baseline under testdata/baseline is the LAST RELEASED contract — a verbatim
// copy of contract/{manifest,schema}.json as of the previous protocol version. The
// differ answers one question about the pair: would a client written against the
// baseline still work? Everything it calls breaking is something such a client fails
// on, not something a reviewer finds distasteful.
//
// Two artifacts, not three. The manifest is the authority on the method, capability,
// error and union surface; the schema bundle on shapes. openrpc.json restates both,
// so reading it would give the diff a second opinion about facts it already has. The
// manifest sections the differ skips are projections of the ones it reads — the run
// event policy comes from the stream union, the state policy from a registered shape
// — and diffing those too would report one change twice.
//
// Cutting a release means re-snapshotting the baseline. Until then these gates keep
// answering the two questions a version number exists to answer: was the bump
// earned, and was it taken?

// closedSetRationale is why gaining a member is as breaking as losing one. The
// frontend declares these unions itself and its fold is exhaustive — a member with no
// branch fails to compile — which is the whole reason the contract distinguishes a
// closed set from an open one.
const closedSetRationale = "a closed set: an exhaustive client has no branch for a member it never saw"

type compatibility struct {
	breaking   []string
	compatible []string
}

func (c *compatibility) breaks(format string, args ...any) {
	c.breaking = append(c.breaking, fmt.Sprintf(format, args...))
}

func (c *compatibility) allows(format string, args ...any) {
	c.compatible = append(c.compatible, fmt.Sprintf(format, args...))
}

// releaseManifest is the slice of the published manifest the differ reads.
type releaseManifest struct {
	Protocol struct {
		Current      string `json:"current"`
		MinSupported string `json:"minSupported"`
	} `json:"protocol"`
	Methods          []manifestMethod `json:"methods"`
	Notifications    []string         `json:"notifications"`
	CapabilityPolicy []manifestPolicy `json:"capabilityPolicy"`
	Errors           manifestErrors   `json:"errors"`
	RuntimeTopics    []manifestTopic  `json:"runtimeTopics"`
	Unions           []manifestUnion  `json:"unions"`
}

type manifestMethod struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type manifestPolicy struct {
	Method string               `json:"method"`
	Rules  []manifestPolicyRule `json:"rules"`
}

type manifestPolicyRule struct {
	Requires []string `json:"requires"`
}

type manifestErrors struct {
	Types []manifestError `json:"types"`
}

type manifestError struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

type manifestTopic struct {
	Type string `json:"type"`
}

type manifestUnion struct {
	Type     string            `json:"type"`
	Variants []manifestVariant `json:"variants"`
}

type manifestVariant struct {
	Tag      string   `json:"tag"`
	Required []string `json:"required"`
}

type releaseShapes struct {
	Defs map[string]releaseShape `json:"$defs"`
}

type releaseShape struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
	Enum       []string                   `json:"enum"`
}

// diffContract classifies every difference between two releases of the contract.
func diffContract(before, after releaseManifest, beforeShapes, afterShapes releaseShapes) compatibility {
	var out compatibility
	diffProtocolRange(before, after, &out)
	diffMethods(before, after, &out)
	diffCapabilityPolicy(before, after, &out)
	diffErrors(before, after, &out)
	diffRuntimeTopics(before, after, &out)
	diffUnions(before, after, &out)
	diffShapes(beforeShapes, afterShapes, &out)
	return out
}

func diffProtocolRange(before, after releaseManifest, out *compatibility) {
	if before.Protocol.MinSupported != after.Protocol.MinSupported {
		out.breaks("minSupported moved from %s to %s, so a client on the old minimum is refused",
			before.Protocol.MinSupported, after.Protocol.MinSupported)
	}
}

func diffMethods(before, after releaseManifest, out *compatibility) {
	kinds := func(m releaseManifest) map[string]string {
		byName := make(map[string]string, len(m.Methods))
		for _, method := range m.Methods {
			byName[method.Name] = method.Kind
		}
		return byName
	}
	beforeKinds, afterKinds := kinds(before), kinds(after)
	for _, name := range slices.Sorted(maps.Keys(beforeKinds)) {
		kind, kept := afterKinds[name]
		switch {
		case !kept:
			out.breaks("method %s is gone", name)
		case kind != beforeKinds[name]:
			out.breaks("method %s changed from %s to %s", name, beforeKinds[name], kind)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(afterKinds)) {
		if _, existed := beforeKinds[name]; !existed {
			out.allows("method %s is new", name)
		}
	}
	// A notification is a method the runtime CALLS, so the client implements it:
	// dropping one strands a subscriber, adding one cannot.
	for _, name := range before.Notifications {
		if !slices.Contains(after.Notifications, name) {
			out.breaks("notification %s is gone", name)
		}
	}
}

func diffCapabilityPolicy(before, after releaseManifest, out *compatibility) {
	requirements := func(m releaseManifest) map[string][]string {
		byMethod := make(map[string][]string, len(m.CapabilityPolicy))
		for _, entry := range m.CapabilityPolicy {
			var features []string
			for _, rule := range entry.Rules {
				features = append(features, rule.Requires...)
			}
			slices.Sort(features)
			byMethod[entry.Method] = slices.Compact(features)
		}
		return byMethod
	}
	beforeRules, afterRules := requirements(before), requirements(after)
	for _, method := range slices.Sorted(maps.Keys(afterRules)) {
		for _, feature := range afterRules[method] {
			if !slices.Contains(beforeRules[method], feature) {
				out.breaks("%s now requires feature %s, which an old client does not declare", method, feature)
			}
		}
	}
	for _, method := range slices.Sorted(maps.Keys(beforeRules)) {
		for _, feature := range beforeRules[method] {
			if !slices.Contains(afterRules[method], feature) {
				out.allows("%s no longer requires feature %s", method, feature)
			}
		}
	}
}

func diffErrors(before, after releaseManifest, out *compatibility) {
	codes := func(m releaseManifest) map[string]int {
		byType := make(map[string]int, len(m.Errors.Types))
		for _, entry := range m.Errors.Types {
			byType[entry.Type] = entry.Code
		}
		return byType
	}
	beforeCodes, afterCodes := codes(before), codes(after)
	for _, kind := range slices.Sorted(maps.Keys(beforeCodes)) {
		code, kept := afterCodes[kind]
		switch {
		case !kept:
			out.breaks("error %s is gone, so a client branching on it never matches again", kind)
		case code != beforeCodes[kind]:
			out.breaks("error %s changed code from %d to %d", kind, beforeCodes[kind], code)
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(afterCodes)) {
		if _, existed := beforeCodes[kind]; !existed {
			out.allows("error %s is new", kind)
		}
	}
}

// diffRuntimeTopics is gate 17's subject. The topic set is closed AND a subscription
// names its topics, so both directions break: a member the client cannot fold, or a
// topic it asks for and the runtime no longer serves.
func diffRuntimeTopics(before, after releaseManifest, out *compatibility) {
	names := func(m releaseManifest) []string {
		topics := make([]string, 0, len(m.RuntimeTopics))
		for _, topic := range m.RuntimeTopics {
			topics = append(topics, topic.Type)
		}
		return topics
	}
	beforeTopics, afterTopics := names(before), names(after)
	for _, topic := range afterTopics {
		if !slices.Contains(beforeTopics, topic) {
			out.breaks("runtime topic %s is new — %s", topic, closedSetRationale)
		}
	}
	for _, topic := range beforeTopics {
		if !slices.Contains(afterTopics, topic) {
			out.breaks("runtime topic %s is gone, so a subscription naming it is refused", topic)
		}
	}
}

func diffUnions(before, after releaseManifest, out *compatibility) {
	variants := func(m releaseManifest) map[string]map[string][]string {
		byType := make(map[string]map[string][]string, len(m.Unions))
		for _, union := range m.Unions {
			members := make(map[string][]string, len(union.Variants))
			for _, variant := range union.Variants {
				members[variant.Tag] = variant.Required
			}
			byType[union.Type] = members
		}
		return byType
	}
	beforeUnions, afterUnions := variants(before), variants(after)
	for _, name := range slices.Sorted(maps.Keys(beforeUnions)) {
		members, kept := afterUnions[name]
		if !kept {
			out.breaks("union %s is gone", name)
			continue
		}
		for _, tag := range slices.Sorted(maps.Keys(beforeUnions[name])) {
			required, present := members[tag]
			if !present {
				out.breaks("union %s lost variant %s", name, tag)
				continue
			}
			for _, field := range required {
				if !slices.Contains(beforeUnions[name][tag], field) {
					out.breaks("union %s variant %s now requires %s", name, tag, field)
				}
			}
		}
		for _, tag := range slices.Sorted(maps.Keys(members)) {
			if _, existed := beforeUnions[name][tag]; !existed {
				out.breaks("union %s gained variant %s — %s", name, tag, closedSetRationale)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(afterUnions)) {
		if _, existed := beforeUnions[name]; !existed {
			out.allows("union %s is new", name)
		}
	}
}

func diffShapes(before, after releaseShapes, out *compatibility) {
	for _, name := range slices.Sorted(maps.Keys(before.Defs)) {
		current, kept := after.Defs[name]
		if !kept {
			out.breaks("shape %s is gone", name)
			continue
		}
		previous := before.Defs[name]
		diffShapeProperties(name, previous, current, out)
		// An enum is closed for the same reason a union is: the client switches on it.
		for _, value := range previous.Enum {
			if !slices.Contains(current.Enum, value) {
				out.breaks("%s no longer accepts %s", name, value)
			}
		}
		for _, value := range current.Enum {
			if !slices.Contains(previous.Enum, value) {
				out.breaks("%s gained value %s — %s", name, value, closedSetRationale)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(after.Defs)) {
		if _, existed := before.Defs[name]; !existed {
			out.allows("shape %s is new", name)
		}
	}
}

func diffShapeProperties(name string, previous, current releaseShape, out *compatibility) {
	for _, field := range slices.Sorted(maps.Keys(previous.Properties)) {
		replacement, present := current.Properties[field]
		switch {
		case !present:
			out.breaks("%s.%s is gone", name, field)
		// Both sides come out of the same emitter with sorted keys, so the encoded
		// property IS the declaration: comparing the bytes catches a changed type, a
		// changed format and a re-pointed $ref without enumerating what a schema may say.
		case string(replacement) != string(previous.Properties[field]):
			out.breaks("%s.%s changed from %s to %s", name, field,
				string(previous.Properties[field]), string(replacement))
		}
	}
	for _, field := range slices.Sorted(maps.Keys(current.Properties)) {
		if _, existed := previous.Properties[field]; existed {
			continue
		}
		if slices.Contains(current.Required, field) {
			out.breaks("%s.%s is new AND required, so an old client's request is rejected", name, field)
			continue
		}
		out.allows("%s.%s is a new optional field", name, field)
	}
	for _, field := range current.Required {
		if _, existed := previous.Properties[field]; existed && !slices.Contains(previous.Required, field) {
			out.breaks("%s.%s became required", name, field)
		}
	}
}

// TestReleaseIsBreaking is contract §11.4 gate 16: the differ classifies this release
// as breaking.
//
// The assertion is not "something is breaking" — one renamed field would satisfy
// that. vNext reshaped the method surface, the closed topic set, the error registry,
// the shapes and several unions, and each has to show up, because a differ blind to
// one category would pass this release while ignoring that category forever.
func TestReleaseIsBreaking(t *testing.T) {
	diff := diffReleases(t)
	if len(diff.breaking) == 0 {
		t.Fatal("the differ found nothing breaking between the last release and this one")
	}
	for _, category := range []struct{ name, needle string }{
		{"the method surface", "method "},
		{"the closed topic set", "runtime topic "},
		{"the error registry", "error "},
		{"the shapes", "shape "},
		{"a union's variants", "union "},
	} {
		if !slices.ContainsFunc(diff.breaking, func(entry string) bool { return strings.Contains(entry, category.needle) }) {
			t.Errorf("the diff reports nothing breaking about %s; vNext changed it", category.name)
		}
	}
	if len(diff.compatible) == 0 {
		t.Error("the differ classified nothing as compatible; a rule set that can only say 'breaking' is not a classifier")
	}
	t.Logf("%d breaking, %d compatible", len(diff.breaking), len(diff.compatible))
}

// TestBreakingChangesRequireAVersionBump is gate 17.
//
// The topic set is the case the contract names: a new closed RuntimeTopic is breaking
// and must be paid for with a protocol version. The rule is stated in general,
// though — ANY breaking change requires the bump — because a gate that only watched
// topics would let the next release ship a removed method for free.
func TestBreakingChangesRequireAVersionBump(t *testing.T) {
	before, after, beforeShapes, afterShapes := readReleases(t)
	diff := diffContract(before, after, beforeShapes, afterShapes)

	newTopic := slices.ContainsFunc(diff.breaking, func(entry string) bool {
		return strings.HasPrefix(entry, "runtime topic ") && strings.Contains(entry, " is new")
	})
	if !newTopic {
		t.Error("this release added runtime topics and the diff does not call that breaking")
	}
	if len(diff.breaking) == 0 {
		return
	}
	if before.Protocol.Current == after.Protocol.Current {
		t.Errorf("the contract changed in %d breaking ways and still says protocol %s:\n  %s",
			len(diff.breaking), after.Protocol.Current, strings.Join(diff.breaking, "\n  "))
	}
}

// TestDifferCallsAnUnchangedContractCompatible is the counter-example the two gates
// above need. Without it a differ that answered "breaking" to everything would pass
// them both, and the release would be certified by a function that cannot say no.
func TestDifferCallsAnUnchangedContractCompatible(t *testing.T) {
	_, after, _, afterShapes := readReleases(t)
	diff := diffContract(after, after, afterShapes, afterShapes)
	if len(diff.breaking) != 0 {
		t.Errorf("the differ called a contract incompatible with itself:\n  %s", strings.Join(diff.breaking, "\n  "))
	}
	if len(diff.compatible) != 0 {
		t.Errorf("the differ found changes between a contract and itself:\n  %s", strings.Join(diff.compatible, "\n  "))
	}
}

// TestDifferSeparatesAdditiveChangesFromBreakingOnes pins the classification itself,
// one synthetic pair per rule. The release diff cannot do this: it is a single pair,
// so it only exercises whichever rules vNext happened to trip.
func TestDifferSeparatesAdditiveChangesFromBreakingOnes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(*releaseManifest, *releaseShapes)
		breaking bool
	}{
		{
			name: "a new method",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Methods = append(m.Methods, manifestMethod{Name: "things.list", Kind: "unary"})
			},
		},
		{
			name:     "a removed method",
			mutate:   func(m *releaseManifest, _ *releaseShapes) { m.Methods = m.Methods[:len(m.Methods)-1] },
			breaking: true,
		},
		{
			name: "a method that became streaming",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Methods[0].Kind = "stream"
			},
			breaking: true,
		},
		{
			name: "a new optional field",
			mutate: func(_ *releaseManifest, s *releaseShapes) {
				setProperty(s, "Thing", "nickname", `{"type":"string"}`)
			},
		},
		{
			name: "a new required field",
			mutate: func(_ *releaseManifest, s *releaseShapes) {
				setProperty(s, "Thing", "nickname", `{"type":"string"}`)
				require(s, "Thing", "nickname")
			},
			breaking: true,
		},
		{
			name:     "an existing field becoming required",
			mutate:   func(_ *releaseManifest, s *releaseShapes) { require(s, "Thing", "label") },
			breaking: true,
		},
		{
			name: "a retyped field",
			mutate: func(_ *releaseManifest, s *releaseShapes) {
				setProperty(s, "Thing", "label", `{"type":"integer"}`)
			},
			breaking: true,
		},
		{
			name: "a removed field",
			mutate: func(_ *releaseManifest, s *releaseShapes) {
				delete(s.Defs["Thing"].Properties, "label")
			},
			breaking: true,
		},
		{
			name: "a new closed enum value",
			mutate: func(_ *releaseManifest, s *releaseShapes) {
				def := s.Defs["Mood"]
				def.Enum = append(def.Enum, "wistful")
				s.Defs["Mood"] = def
			},
			breaking: true,
		},
		{
			name: "a new runtime topic",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.RuntimeTopics = append(m.RuntimeTopics, manifestTopic{Type: "things.changed"})
			},
			breaking: true,
		},
		{
			name: "a new union variant",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Unions[0].Variants = append(m.Unions[0].Variants, manifestVariant{Tag: "postponed"})
			},
			breaking: true,
		},
		{
			name: "a new capability requirement",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.CapabilityPolicy[0].Rules[0].Requires = append(m.CapabilityPolicy[0].Rules[0].Requires, "telepathy")
			},
			breaking: true,
		},
		{
			name: "a dropped capability requirement",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.CapabilityPolicy[0].Rules[0].Requires = nil
			},
		},
		{
			name: "a new error type",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Errors.Types = append(m.Errors.Types, manifestError{Type: "thing_not_found", Code: -32099})
			},
		},
		{
			name: "a renumbered error",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Errors.Types[0].Code = -32098
			},
			breaking: true,
		},
		{
			name: "a raised minimum version",
			mutate: func(m *releaseManifest, _ *releaseShapes) {
				m.Protocol.MinSupported = "2030-01-01"
			},
			breaking: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before, beforeShapes := syntheticContract()
			after, afterShapes := syntheticContract()
			tt.mutate(&after, &afterShapes)

			diff := diffContract(before, after, beforeShapes, afterShapes)
			switch {
			case tt.breaking && len(diff.breaking) == 0:
				t.Errorf("the differ let %s through as compatible", tt.name)
			case !tt.breaking && len(diff.breaking) != 0:
				t.Errorf("the differ called %s breaking: %s", tt.name, strings.Join(diff.breaking, "; "))
			case !tt.breaking && len(diff.compatible) == 0:
				t.Errorf("the differ did not notice %s at all", tt.name)
			}
		})
	}
}

// syntheticContract is a contract small enough to mutate one rule at a time. Two
// calls produce two independent copies, so a mutation cannot reach the "before" side
// through a shared slice or map.
func syntheticContract() (releaseManifest, releaseShapes) {
	document := releaseManifest{
		Methods:       []manifestMethod{{Name: "things.get", Kind: "unary"}},
		Notifications: []string{"notifications.thing.event"},
		CapabilityPolicy: []manifestPolicy{{
			Method: "things.get",
			Rules:  []manifestPolicyRule{{Requires: []string{"things"}}},
		}},
		Errors:        manifestErrors{Types: []manifestError{{Type: "thing_missing", Code: -32001}}},
		RuntimeTopics: []manifestTopic{{Type: "things.stirred"}},
		Unions: []manifestUnion{{
			Type:     "ThingOutcome",
			Variants: []manifestVariant{{Tag: "done"}, {Tag: "failed", Required: []string{"error"}}},
		}},
	}
	document.Protocol.Current = "2026-01-01"
	document.Protocol.MinSupported = "2026-01-01"
	shapes := releaseShapes{Defs: map[string]releaseShape{
		"Thing": {
			Properties: map[string]json.RawMessage{
				"id":    json.RawMessage(`{"type":"string"}`),
				"label": json.RawMessage(`{"type":"string"}`),
			},
			Required: []string{"id"},
		},
		"Mood": {Enum: []string{"calm", "brisk"}},
	}}
	return document, shapes
}

func setProperty(shapes *releaseShapes, shape, field, declaration string) {
	shapes.Defs[shape].Properties[field] = json.RawMessage(declaration)
}

func require(shapes *releaseShapes, shape, field string) {
	def := shapes.Defs[shape]
	def.Required = append(def.Required, field)
	shapes.Defs[shape] = def
}

func diffReleases(t *testing.T) compatibility {
	t.Helper()
	before, after, beforeShapes, afterShapes := readReleases(t)
	return diffContract(before, after, beforeShapes, afterShapes)
}

func readReleases(t *testing.T) (before, after releaseManifest, beforeShapes, afterShapes releaseShapes) {
	t.Helper()
	root := moduleRoot(t)
	baseline := filepath.Join(root, "internal", "arch", "testdata", "baseline")
	current := filepath.Join(root, "contract")

	decodeRelease(t, readArtifact(t, baseline, "manifest.json"), &before)
	decodeRelease(t, readArtifact(t, current, "manifest.json"), &after)
	decodeRelease(t, readArtifact(t, baseline, "schema.json"), &beforeShapes)
	decodeRelease(t, readArtifact(t, current, "schema.json"), &afterShapes)

	if len(before.Methods) == 0 || len(after.Methods) == 0 || len(beforeShapes.Defs) == 0 || len(afterShapes.Defs) == 0 {
		t.Fatal("a release snapshot decoded to nothing; the diff would be comparing emptiness")
	}
	return before, after, beforeShapes, afterShapes
}

func decodeRelease(t *testing.T, raw []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode release snapshot: %v", err)
	}
}
