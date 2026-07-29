package dispatch

import (
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// shapes is the registered union / constraint / state-key contract.
//
// Contract §11.2 names thirteen high-risk unions. Three of them — ItemListScope,
// CapabilityRequirement, CancelRunResponse — do not exist yet: they arrive later
// in the vNext cutover (C5 / C6 / C8). They are registered when their types land,
// not now with an invented shape; a spec for a type nobody can send is a spec
// nothing can check.
var shapes = buildShapes()

func buildShapes() *Shapes {
	s := &Shapes{}
	registerRunUnions(s)
	registerItemUnions(s)
	registerInterruptUnions(s)
	registerEventUnions(s)
	registerArtifactUnions(s)
	registerDiffUnions(s)
	registerObjectConstraints(s)
	registerStateKeys(s)
	registerCarriedShapes(s)
	registerValueConstraints(s)
	return s
}

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

func registerRunUnions(s *Shapes) {
	// A terminal says only why the run stopped; what it consumed is published
	// beside it as metrics. `detail` is the human-readable note the non-error
	// terminals may add (§4.2) — the error terminal's note stays on
	// error.detail, never duplicated here.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.RunOutcome](),
		Discriminator: "type",
		Variants:      runOutcomeVariants(),
	})

	// A segment stops for any reason a run does, plus the two that leave the run
	// alive. The terminal variants are the SAME list, converted — because
	// SegmentOutcome contains RunOutcome, and a second list is how a terminal comes
	// to be legal for one and not the other.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.SegmentOutcome](),
		Discriminator: "type",
		Variants: append([]VariantSpec{
			{Tag: string(protocol.SegmentInterrupt), Required: []string{"interrupts"}},
			// `suspended` adds nothing: the interrupts belong to the run that raised
			// them, so a run stopped by someone else's barrier carries none.
			{Tag: string(protocol.SegmentSuspended)},
		}, runOutcomeVariants()...),
	})
}

// runOutcomeVariants is the terminal half of both run-outcome unions. It is a
// function rather than a shared slice because a VariantSpec holds slices a caller
// could otherwise append into, and the two registrations must not be able to
// reach each other's fields.
func runOutcomeVariants() []VariantSpec {
	return []VariantSpec{
		{Tag: string(protocol.OutcomeCompleted)},
		{Tag: string(protocol.OutcomeError), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeMaxSteps), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeMaxBudget), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeCanceled), Optional: []string{"detail"}},
	}
}

func registerItemUnions(s *Shapes) {
	// The base fields (id / runId / status / createdAt) are on every variant, so
	// each variant repeats them: a variant declares the WHOLE frame it permits,
	// which is what lets the field-coverage check be exhaustive.
	base := []string{"id", "runId", "status", "createdAt"}
	s.union(UnionSpec{
		GoType:        typeOf[protocol.Item](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemTypeUserMessage), Required: base, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeAgentMessage), Required: base, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeReasoning), Required: base, Optional: []string{"text", "redacted"}},
			{Tag: string(protocol.ItemTypePlan), Required: base, Optional: []string{"steps"}},
			{Tag: string(protocol.ItemTypeQuestion), Required: base, Optional: []string{"question"}},
			{Tag: string(protocol.ItemTypeToolCall), Required: base, Optional: []string{"tool", "safetyClass", "error"}},
			{Tag: string(protocol.ItemTypeCompaction), Required: base, Optional: []string{"summary", "droppedMessages"}},
		},
	})

	// Every delta is ephemeral and every one has a named durable landing
	// (API.md §5.2). toolArguments is partial JSON TEXT, not an object — the
	// parsed value only exists on the completed item.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ItemDelta](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.DeltaContent), Required: []string{"text"}, Optional: []string{"index"}},
			{Tag: string(protocol.DeltaReasoning), Required: []string{"text"}},
			{Tag: string(protocol.DeltaToolArguments), Required: []string{"argumentsTextDelta"}},
			{Tag: string(protocol.DeltaToolOutput), Required: []string{"text"}},
			{Tag: string(protocol.DeltaPlan), Required: []string{"steps"}},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.ContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ContentBlockText), Required: []string{"text"}},
			// Images are inline: mime + raw base64, no data: prefix, no upload channel.
			{Tag: string(protocol.ContentBlockImage), Required: []string{"mime", "data"}},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.QuestionField](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.QuestionFieldText), Required: []string{"name", "label"}, Optional: []string{"header", "required"}},
			{Tag: string(protocol.QuestionFieldChoice), Required: []string{"name", "label", "options"}, Optional: []string{"header", "required", "multiple"}},
		},
	})
}

func registerInterruptUnions(s *Shapes) {
	// The variant fields live inside `payload`, so the spec addresses them by
	// dotted path. Each of the three is self-contained — API.md §4.8's whole point
	// is that rendering a pending interrupt never needs a second request.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.Interrupt](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptApproval),
				Required: []string{"itemId", "payload.tool"},
				Optional: []string{"payload.risk", "payload.reason", "payload.rememberable"},
			},
			{Tag: string(protocol.InterruptQuestion), Required: []string{"itemId", "payload.question"}},
			{Tag: string(protocol.InterruptToolResult), Required: []string{"itemId", "payload.tool"}},
		},
	})

	// editedArgs is one-shot by design: a remembered rule matches by the call's
	// subject, never by a one-off argument rewrite (AUX_API §6).
	s.union(UnionSpec{
		GoType:        typeOf[protocol.InterruptResponseValue](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptResponseApproval),
				Required: []string{"decision"},
				Optional: []string{"remember", "editedArgs", "reason"},
			},
			{Tag: string(protocol.InterruptResponseAnswer), Required: []string{"answers"}},
			// A client tool reports either a result or a failure, so both are optional
			// individually; "exactly one" is a presence rule, not a variant field list.
			{Tag: string(protocol.InterruptResponseToolResult), Optional: []string{"result", "error"}},
		},
	})
}

func registerEventUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.StreamEvent](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.StreamSegmentStarted), Required: []string{"run"}},
			{Tag: string(protocol.StreamSegmentProgress), Required: []string{"progress"}},
			{Tag: string(protocol.StreamSegmentFinished), Required: []string{"outcome", "metrics"}},
			{Tag: string(protocol.StreamItemStarted), Required: []string{"item"}},
			{Tag: string(protocol.StreamItemDelta), Required: []string{"itemId", "delta"}},
			{Tag: string(protocol.StreamItemCompleted), Required: []string{"item"}},
			{Tag: string(protocol.StreamStateSnapshot), Required: []string{"state"}},
			// `custom` is the only event whose durability is not a function of its
			// type, so it is the only one carrying the flag (API.md §5.2).
			{Tag: string(protocol.StreamCustom), Required: []string{"name"}, Optional: []string{"payload", "durable"}},
		},
	})

	// Today's non-run failure stream. vNext replaces it with the nine-topic
	// RuntimeEvent (C12) and strips the payload facts from mcp.serverChanged;
	// this is the current shape, registered so the drift gate has a baseline.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.WorkspaceEvent](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.WorkspaceEventFilesChanged), Required: []string{"sequence", "paths"}, Optional: []string{"watchId", "cwd"}},
			{Tag: string(protocol.WorkspaceEventSkillsChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.WorkspaceEventMCPServerChanged), Required: []string{"sequence", "server"}, Optional: []string{"status", "toolCount", "error"}},
			{Tag: string(protocol.WorkspaceEventSchedulesFired), Required: []string{"sequence", "scheduleId"}},
			{Tag: string(protocol.WorkspaceEventResync), Required: []string{"sequence"}},
		},
	})
}

func registerArtifactUnions(s *Shapes) {
	// An artifact's terminal vocabulary is deliberately NOT the live RunOutcome
	// union: it cannot carry the live-only interrupt outcome, because a parked
	// executor is process-local and does not travel.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactOutcome](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ArtifactOutcomeCompleted)},
			{Tag: string(protocol.ArtifactOutcomeError), Required: []string{"error"}},
			{Tag: string(protocol.ArtifactOutcomeMaxSteps), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeMaxBudget), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeCanceled), Optional: []string{"detail"}},
		},
	})

	base := []string{"id", "runId", "status", "createdAt"}
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactItem](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemTypeUserMessage), Required: base, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeAgentMessage), Required: base, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeReasoning), Required: base, Optional: []string{"text", "redacted"}},
			{Tag: string(protocol.ItemTypePlan), Required: base, Optional: []string{"steps"}},
			{Tag: string(protocol.ItemTypeQuestion), Required: base, Optional: []string{"question"}},
			{Tag: string(protocol.ItemTypeToolCall), Required: base, Optional: []string{"tool", "safetyClass", "error"}},
			{Tag: string(protocol.ItemTypeCompaction), Required: base, Optional: []string{"summary", "droppedMessages"}},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ContentBlockText), Required: []string{"text"}},
			{Tag: string(protocol.ContentBlockImage), Required: []string{"mime", "data"}},
		},
	})
}

func registerDiffUnions(s *Shapes) {
	// A diff row's godoc has always described a union — a hunk carries text, a context
	// row carries both line numbers, an added row only the right one — and the frontend
	// modeled it as one. Nothing said so on the wire, so the generated shape permitted
	// a row carrying a hunk's text AND both line numbers at once.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.DiffRow](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.DiffRowHunk), Required: []string{"text"}},
			// The line numbers are `omitempty` because one flat struct serves four
			// tags, so an added row must be able to drop leftLine. They are REQUIRED
			// here anyway: a context row carries both, and a unified diff numbers
			// lines from 1, so the zero omitempty drops never occurs.
			{Tag: string(protocol.DiffRowContext), Required: []string{"code", "leftLine", "rightLine"}},
			{Tag: string(protocol.DiffRowAdded), Required: []string{"code", "rightLine"}},
			{Tag: string(protocol.DiffRowDeleted), Required: []string{"code", "leftLine"}},
		},
	})
}

func registerObjectConstraints(s *Shapes) {
	// A finished Run explains itself, and a run that has not finished does not
	// pretend to. Without the first rule `status:"finished"` with no outcome is
	// representable and a client cannot tell "it ended" from "it ended somehow";
	// without the others, a waiting run could carry a terminal reason and a client
	// would stop offering to resume it (§4.2).
	//
	// These are the SUMMARY's rules, so they hold wherever a summary travels — the
	// page-level runs of items.list as much as a RunRef, which embeds it.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunSummary](),
		Rules: append([]PresenceRule{{
			When:     []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusFinished)}},
			Required: []string{"outcome", "finishedAt"},
		}, {
			When:      []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusRunning)}},
			Forbidden: []string{"outcome", "finishedAt"},
		}, {
			When:      []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusWaiting)}},
			Forbidden: []string{"outcome", "finishedAt"},
		}}, childLineageRules()...),
	})

	// A RunRef adds the control field, and it exists exactly while a segment is
	// executing: without the first rule a running run can arrive with nothing to
	// attach to, and without the second a client can attach to a stream that
	// already ended (§4.1).
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunRef](),
		Rules: []PresenceRule{{
			When:     []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusRunning)}},
			Required: []string{"activeSegmentId"},
		}, {
			When:      []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusWaiting)}},
			Forbidden: []string{"activeSegmentId"},
		}, {
			When:      []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusFinished)}},
			Forbidden: []string{"activeSegmentId"},
		}},
	})

	// An error terminal's explanation lives on `error`, and only there — `detail`
	// is the note the OTHER terminals may add, so carrying both would give one
	// failure two prose fields and let them disagree.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunOutcome](),
		Rules:  []PresenceRule{errorTerminalRule()},
	})

	// The same rule for the segment union, plus the two non-terminal stops: an
	// interrupt has something to resume, and a suspended segment carries no
	// interrupts because they belong to the run that raised them.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.SegmentOutcome](),
		Rules: []PresenceRule{errorTerminalRule(), {
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.SegmentInterrupt)}},
			Required:  []string{"interrupts"},
			Forbidden: []string{"error", "detail"},
		}, {
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.SegmentSuspended)}},
			Forbidden: []string{"interrupts", "error", "detail"},
		}},
	})

	// A pending set with no interrupts is not a thing to resume — it would leave
	// the client polling a run that will never move (contract §11.2).
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.OpenInterrupt](),
		Rules: []PresenceRule{{
			Required: []string{"runId", "sessionId", "interrupts", "createdAt"},
		}},
	})

	// An error outcome's explanation lives on `error`, and only there — the
	// archive's copy of the live rule, because an exported run has to say why it
	// failed as unambiguously as a live one.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactOutcome](),
		Rules: []PresenceRule{{
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.ArtifactOutcomeError)}},
			Required:  []string{"error"},
			Forbidden: []string{"detail"},
		}},
	})
}

// childLineageRules say the three child edges are all-or-none: a run either
// carries every one of them or is a root (§4.2). Stated as one rule per edge
// rather than "root forbids them", because presence is the only thing a
// PresenceRule can condition on and each edge is the condition for the other two.
//
// The contract's other half — that neither RunId equals the run's own id — is
// NOT here. JSON Schema cannot compare two fields, so it could not be one of the
// three equivalent statements §11.2 asks for; it is an identity invariant of the
// child-creation transaction, which does not exist while features.subagents is
// off. It belongs in SystemInvariantSpec when that transaction lands, and fusing
// an inequality into a presence rule would be one primitive doing two jobs.
func childLineageRules() []PresenceRule {
	edges := []string{"spawnedByItemId", "parentRunId", "rootRunId"}
	rules := make([]PresenceRule, 0, len(edges))
	for index, edge := range edges {
		others := append(append([]string{}, edges[:index]...), edges[index+1:]...)
		rules = append(rules, PresenceRule{
			When:     []FieldCondition{{Field: edge, Operator: OperatorPresent}},
			Required: others,
		})
	}
	return rules
}

// errorTerminalRule is the error terminal's shape, stated once for both unions
// that contain it.
func errorTerminalRule() PresenceRule {
	return PresenceRule{
		When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeError)}},
		Required:  []string{"error"},
		Forbidden: []string{"detail"},
	}
}

func registerStateKeys(s *Shapes) {
	// `todos` is the only first-party shared-state key today.
	//
	// Its recovery method is runs.subscribe: the state.snapshot is durable and
	// replayed on re-subscribe, so there is deliberately no cold-read RPC that
	// could drift from the event projection (API.md appendix C.4). vNext adds
	// todos.get and moves recovery there (C7) — at which point THIS line changes,
	// which is exactly the drift a spec is supposed to make visible.
	s.stateKey(StateKeySpec{
		Key:            "todos",
		RecoveryMethod: "runs.subscribe",
		Scope:          StateScopeSession,
		Writer:         StateWriterRootRun,
		Feature:        "todos",
		Stability:      stable,
		PayloadType:    typeOf[[]protocol.TodoSnapshot](),
	})
}

func registerCarriedShapes(s *Shapes) {
	// `params._meta` is stripped before typed params are decoded, so the walk cannot
	// reach it — yet every client constructs it (API.md §2.4).
	s.carriedShape(CarriedSpec{Carrier: "params._meta", GoType: typeOf[protocol.RequestMeta]()})

	// A tool result is `any` on purpose: the runtime does not constrain what a tool
	// returns. These are the shapes first-party tools DO return inside it (API.md
	// §4.4.2), and the client renders them, so their shapes are published even though
	// the carrier is opaque. Which tool returns which is documented in §4.4.2 and not
	// declared here: the per-tool result wrapper objects have no Go types yet, and
	// inventing the binding without them would be a guess.
	for _, carried := range []reflect.Type{
		typeOf[protocol.FileEdit](),
		typeOf[protocol.SearchHit](),
		typeOf[protocol.WebSearchResult](),
	} {
		s.carriedShape(CarriedSpec{Carrier: "items[].tool.result", GoType: carried})
	}
}
