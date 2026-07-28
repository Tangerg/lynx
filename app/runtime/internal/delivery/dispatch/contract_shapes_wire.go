package dispatch

import (
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// shapes is the registered union / constraint / state-key contract.
//
// Contract §11.2 names thirteen high-risk unions. Four of them —
// SegmentOutcome, ItemListScope, CapabilityRequirement, CancelRunResponse — do
// not exist yet: they arrive with the vNext cutover (C1 / C5 / C6). They are
// registered when their types land, not now with an invented shape; a spec for a
// type nobody can send is a spec nothing can check.
var shapes = buildShapes()

func buildShapes() *Shapes {
	s := &Shapes{}
	registerRunUnions(s)
	registerItemUnions(s)
	registerInterruptUnions(s)
	registerEventUnions(s)
	registerArtifactUnions(s)
	registerObjectConstraints(s)
	registerStateKeys(s)
	return s
}

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

func registerRunUnions(s *Shapes) {
	// Every terminal but `interrupt` carries metering; `interrupt` carries the
	// pending set instead and is the only RESUMABLE one. `detail` is the
	// human-readable note the non-error terminals may add (API.md §4.2) — the
	// error terminal's note stays on result.error.detail, never duplicated here.
	metered := VariantSpec{Required: []string{"result"}}
	s.union(UnionSpec{
		GoType:        typeOf[protocol.RunOutcome](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.OutcomeCompleted), Required: metered.Required},
			{Tag: string(protocol.OutcomeError), Required: metered.Required},
			{Tag: string(protocol.OutcomeMaxSteps), Required: metered.Required, Optional: []string{"detail"}},
			{Tag: string(protocol.OutcomeMaxBudget), Required: metered.Required, Optional: []string{"detail"}},
			{Tag: string(protocol.OutcomeCanceled), Required: metered.Required, Optional: []string{"detail"}},
			{Tag: string(protocol.OutcomeInterrupt), Required: []string{"interrupts"}},
		},
	})
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
			{Tag: string(protocol.StreamSegmentFinished), Required: []string{"outcome"}},
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
			{Tag: string(protocol.ArtifactOutcomeCompleted), Required: []string{"result"}},
			{Tag: string(protocol.ArtifactOutcomeError), Required: []string{"result"}, Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeMaxSteps), Required: []string{"result"}, Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeMaxBudget), Required: []string{"result"}, Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeCanceled), Required: []string{"result"}, Optional: []string{"detail"}},
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

func registerObjectConstraints(s *Shapes) {
	// A finished Run explains itself. Without this, `status:"finished"` with no
	// outcome is representable, and a client cannot tell "it ended" from "it ended
	// somehow" (API.md §4.2).
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunRef](),
		Rules: []PresenceRule{{
			When:     []FieldCondition{{Field: "status", Operator: OperatorEquals, Value: string(protocol.RunStatusFinished)}},
			Required: []string{"outcome", "finishedAt"},
		}},
	})

	// The metering / interrupt split again, as a presence rule rather than a
	// variant list — the same fact stated for the object validator, which is what
	// rejects a frame that carries both.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunOutcome](),
		Rules: []PresenceRule{{
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeInterrupt)}},
			Required:  []string{"interrupts"},
			Forbidden: []string{"result"},
		}, {
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeError)}},
			Required:  []string{"result"},
			Forbidden: []string{"interrupts", "detail"},
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

	// An error outcome's explanation lives on result.error, and only there.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactOutcome](),
		Rules: []PresenceRule{{
			When:     []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.ArtifactOutcomeError)}},
			Required: []string{"result"},
		}},
	})
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
	})
}
