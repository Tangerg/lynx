package dispatch

import (
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// shapes is the registered union / constraint / state-key contract.
//
// Every high-risk discriminated union in the contract is registered here. The Go
// type, runtime validator, JSON Schema, and generated TypeScript all consume the
// same declaration so a variant cannot drift in only one layer. Extension seams
// remain narrow pattern branches rather than arbitrary strings.
var shapes = buildShapes()

func buildShapes() *Shapes {
	s := &Shapes{}
	registerNotifications(s)
	registerProblemUnion(s)
	registerRunUnions(s)
	registerItemUnions(s)
	registerProviderUnions(s)
	registerMCPUnions(s)
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

func registerProviderUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ProviderConfigChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ProviderConfigSet), Required: []string{"value"}},
			{Tag: string(protocol.ProviderConfigClear)},
		},
	})
}

func registerMCPUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPConnection](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorizationMasked", "headersMasked"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "envMasked", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPConnectionInput](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorization", "headers"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "env", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPAuthorizationChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPHeadersChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPEnvironmentChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPServerState](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPServerDisabled)},
			{Tag: string(protocol.MCPServerDisconnected)},
			{Tag: string(protocol.MCPServerConnecting)},
			{Tag: string(protocol.MCPServerConnected), Required: []string{"toolCount"}},
			{Tag: string(protocol.MCPServerFailed), Required: []string{"error"}},
			{Tag: string(protocol.MCPServerNeedsAuth), Required: []string{"error"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPAuthorizationAttemptStatus](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPAuthorizationAttemptPending)},
			{Tag: string(protocol.MCPAuthorizationAttemptSucceeded)},
			{Tag: string(protocol.MCPAuthorizationAttemptFailed), Required: []string{"error"}},
			{Tag: string(protocol.MCPAuthorizationAttemptCanceled)},
		},
	})
}

func registerProblemUnion(s *Shapes) {
	contracts := ProblemContracts()
	variants := make([]VariantSpec, 0, len(contracts))
	for _, contract := range contracts {
		variants = append(variants, VariantSpec{
			Tag:      contract.Type,
			Required: contract.Required,
			Optional: contract.Optional,
		})
	}
	s.union(UnionSpec{
		GoType: typeOf[protocol.ProblemData](), Discriminator: "type", Variants: variants,
		PatternVariant: &PatternVariantSpec{
			TagPattern:     `^plugin:[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`,
			TypeScriptType: "`plugin:${string}/${string}`",
			Optional:       []string{"detail", "docUrl", "retryAfterSeconds"},
		},
	})
}

func registerNotifications(s *Shapes) {
	s.notification(NotificationSpec{
		Name:       NotificationRunEvent,
		ParamsType: typeOf[protocol.RunEvent](),
	})
	s.notification(NotificationSpec{
		Name:       NotificationRuntimeEvent,
		ParamsType: typeOf[protocol.RuntimeEventNotification](),
	})
}

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

func registerRunUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.CancelRunResponse](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.CancelRunRoot), Required: []string{"run"}},
			{Tag: string(protocol.CancelRunChild), Required: []string{"run", "rootRun"}},
		},
	})

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
		{Tag: string(protocol.OutcomeTimedOut), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeFailed), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeMaxSteps), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeMaxBudget), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeCanceled), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeLost), Required: []string{"error"}},
	}
}

func registerItemUnions(s *Shapes) {
	// Identity/status are shared. Time is intentionally variant-specific:
	// ToolCall uses startedAt, while every other item uses createdAt. A variant
	// declares the WHOLE frame it permits, making the two terms mutually exclusive.
	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	// The archive's session-scoped state values, keyed the same way the live stream
	// keys them. One variant today, declared as a union because the KEY is the
	// discriminator: a reader must branch on it rather than guess from which field
	// happens to be set.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactState](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ArtifactStatePlan), Required: []string{"plan"}},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.Item](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemTypeUserMessage), Required: createdItemFields, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeAgentMessage), Required: createdItemFields, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeReasoning), Required: createdItemFields, Optional: []string{"text", "redacted"}},
			{Tag: string(protocol.ItemTypeQuestion), Required: createdItemFields, Optional: []string{"question"}},
			{Tag: string(protocol.ItemTypeToolCall), Required: toolItemFields, Optional: []string{"finishedAt", "durationMillis", "tool", "safetyClass", "error"}},
			{Tag: string(protocol.ItemTypeCompaction), Required: createdItemFields, Optional: []string{"summary", "droppedMessages"}},
		},
	})

	// Every delta is ephemeral and every one has a named authoritative landing
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
		},
	})

	// Four short registries, so a gap says which vocabulary its name belongs to. The
	// variants carry the same field set on purpose: what differs is what `name` MEANS,
	// and each registry publishes its own values rather than restating them here.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.CapabilityRequirement](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.RequirementFeature), Required: []string{"name"}},
			{Tag: string(protocol.RequirementInterruptType), Required: []string{"name"}},
			{Tag: string(protocol.RequirementRuntimeTopic), Required: []string{"name"}},
			{Tag: string(protocol.RequirementStateSnapshot), Required: []string{"name"}},
		},
	})

	// One key today, and it is tagged: the stream event and the cold read carry the
	// same shape, so a second key must arrive as a new tag rather than as extra
	// optional fields nobody can tell apart.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.StateSnapshot](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.StatePlan),
				Required: []string{"sessionId", "revision", "plan"},
				Optional: []string{"updatedAt"},
			},
		},
	})

	// What a page of items is a page OF. The two subjects are exclusive, not two
	// optional filters: a frame naming both would need a precedence rule to resolve,
	// and a precedence rule is where the request and the answer start to disagree.
	// Only run scope may ask for descendants — the session timeline already holds
	// every descendant, so the flag would narrow nothing there.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ItemListScope](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemScopeSession), Required: []string{"sessionId"}},
			{Tag: string(protocol.ItemScopeRun), Required: []string{"runId"}, Optional: []string{"includeDescendants"}},
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

	for _, fieldType := range []reflect.Type{
		typeOf[protocol.QuestionField](),
		typeOf[protocol.ArtifactQuestionField](),
	} {
		s.union(UnionSpec{
			GoType:        fieldType,
			Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: string(protocol.QuestionFieldText), Required: []string{"prompt"}, Optional: []string{"header"}},
				{Tag: string(protocol.QuestionFieldChoice), Required: []string{"prompt", "options"}, Optional: []string{"header", "multiple", "allowCustom"}},
			},
		})
	}
}

func registerInterruptUnions(s *Shapes) {
	// The variant fields live inside `payload`, so the spec addresses them by
	// dotted path. Each of the three is self-contained — §4.8's whole point is that
	// rendering a pending interrupt never needs a second request — and every one
	// carries the identity pair: which item is waiting, and which run asked.
	identity := []string{"itemId", "runId"}
	s.union(UnionSpec{
		GoType:        typeOf[protocol.Interrupt](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptApproval),
				Required: append(slices.Clone(identity), "payload.tool"),
				Optional: []string{"payload.risk", "payload.reason", "payload.rememberable"},
			},
			{Tag: string(protocol.InterruptQuestion), Required: append(slices.Clone(identity), "payload.question")},
			{Tag: string(protocol.InterruptToolResult), Required: append(slices.Clone(identity), "payload.tool")},
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
		Forbidden:     []string{"durable"},
		Variants: []VariantSpec{
			{Tag: string(protocol.StreamSegmentStarted), Required: []string{"run"}},
			{Tag: string(protocol.StreamSegmentProgress), Required: []string{"progress"}},
			{Tag: string(protocol.StreamSegmentFinished), Required: []string{"outcome", "metrics"}},
			{Tag: string(protocol.StreamItemStarted), Required: []string{"item"}},
			{Tag: string(protocol.StreamItemDelta), Required: []string{"itemId", "delta"}},
			{Tag: string(protocol.StreamItemCompleted), Required: []string{"item"}},
			{Tag: string(protocol.StreamStateSnapshot), Required: []string{"state"}},
			{Tag: string(protocol.StreamCustom), Required: []string{"name"}, Optional: []string{"payload"}},
		},
	})

	// Every variant is an invalidation: `sequence` plus the ids that moved. What a
	// variant may NOT carry is the resource's new value — mcp.changed used to carry a
	// status, a tool count and an error, which made the stream a second source of
	// truth for something mcp.servers answers, and the two drifted the moment a frame
	// was dropped.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.RuntimeEvent](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.RuntimeFilesChanged), Required: []string{"sequence", "paths"}, Optional: []string{"watchId", "workspace"}},
			{Tag: string(protocol.RuntimeSkillsChanged), Required: []string{"sequence"}, Optional: []string{"names"}},
			{Tag: string(protocol.RuntimeMCPChanged), Required: []string{"sequence"}, Optional: []string{"serverIds"}},
			{Tag: string(protocol.RuntimeSchedulesChanged), Required: []string{"sequence"}, Optional: []string{"scheduleIds"}},
			{Tag: string(protocol.RuntimeSessionsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeRunsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			// The key is required: a client holds one projection per key, and a signal
			// that does not say which one asks it to refetch all of them.
			{Tag: string(protocol.RuntimeStateChanged), Required: []string{"sequence", "key"}, Optional: []string{"sessionIds", "runIds"}},
			{Tag: string(protocol.RuntimeGoalsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeInterruptsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			// Resync names what went stale rather than saying "everything": a client that
			// subscribed to nine topics should not reload nine resources because one
			// watch overflowed.
			{Tag: string(protocol.RuntimeResync), Required: []string{"sequence", "topics"}, Optional: []string{"watchIds"}},
		},
	})
}

func registerArtifactUnions(s *Shapes) {
	// An artifact's terminal vocabulary is deliberately NOT the live RunOutcome
	// union: it cannot carry the live-only interrupt outcome, because a parked
	// waiting executor state is Runtime-instance-local and does not travel.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactOutcome](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ArtifactOutcomeCompleted)},
			{Tag: string(protocol.ArtifactOutcomeTimedOut), Required: []string{"error"}},
			{Tag: string(protocol.ArtifactOutcomeFailed), Required: []string{"error"}},
			{Tag: string(protocol.ArtifactOutcomeMaxSteps), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeMaxBudget), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeCanceled), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeLost), Required: []string{"error"}},
		},
	})

	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactItem](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemTypeUserMessage), Required: createdItemFields, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeAgentMessage), Required: createdItemFields, Optional: []string{"content"}},
			{Tag: string(protocol.ItemTypeReasoning), Required: createdItemFields, Optional: []string{"text", "redacted"}},
			{Tag: string(protocol.ItemTypeQuestion), Required: createdItemFields, Optional: []string{"question"}},
			{Tag: string(protocol.ItemTypeToolCall), Required: toolItemFields, Optional: []string{"finishedAt", "durationMillis", "tool", "safetyClass", "error"}},
			{Tag: string(protocol.ItemTypeCompaction), Required: createdItemFields, Optional: []string{"summary", "droppedMessages"}},
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
	// row carries both line numbers, an added row only the right one — and clients
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
	for _, target := range []reflect.Type{
		typeOf[protocol.Item](),
		typeOf[protocol.ArtifactItem](),
	} {
		s.constraint(ObjectConstraintSpec{
			GoType: target,
			Rules: []PresenceRule{{
				When: []FieldCondition{
					{Field: "type", Operator: OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
					{Field: "status", Operator: OperatorEquals, Value: string(protocol.ItemStatusRunning)},
				},
				Forbidden: []string{"finishedAt", "durationMillis"},
			}, {
				When: []FieldCondition{
					{Field: "type", Operator: OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
					{Field: "status", Operator: OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
				},
				Required: []string{"finishedAt", "durationMillis"},
			}, {
				When: []FieldCondition{
					{Field: "type", Operator: OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
					{Field: "status", Operator: OperatorEquals, Value: string(protocol.ItemStatusIncomplete)},
				},
				Required: []string{"finishedAt", "durationMillis"},
			}},
		})
	}

	for _, target := range []reflect.Type{
		typeOf[protocol.AgentMemoryListRequest](),
		typeOf[protocol.AgentMemoryAddRequest](),
	} {
		s.constraint(ObjectConstraintSpec{
			GoType: target,
			Rules: []PresenceRule{{
				When:     []FieldCondition{{Field: "scope", Operator: OperatorEquals, Value: string(protocol.AgentMemoryScopeProject)}},
				Required: []string{"workspace"},
			}, {
				When:      []FieldCondition{{Field: "scope", Operator: OperatorEquals, Value: string(protocol.AgentMemoryScopeUser)}},
				Forbidden: []string{"workspace"},
			}},
		})
	}

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Rules: []PresenceRule{{
			When:      []FieldCondition{{Field: "status.type", Operator: OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptPending)}},
			Forbidden: []string{"finishedAt"},
		}, {
			When:     []FieldCondition{{Field: "status.type", Operator: OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptSucceeded)}},
			Required: []string{"finishedAt"},
		}, {
			When:     []FieldCondition{{Field: "status.type", Operator: OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptFailed)}},
			Required: []string{"finishedAt"},
		}, {
			When:     []FieldCondition{{Field: "status.type", Operator: OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptCanceled)}},
			Required: []string{"finishedAt"},
		}},
	})

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
		Rules:  failureTerminalRules(),
	})

	// The same rule for the segment union, plus the two non-terminal stops: an
	// interrupt has something to resume, and a suspended segment carries no
	// interrupts because they belong to the run that raised them.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.SegmentOutcome](),
		Rules: append(failureTerminalRules(), PresenceRule{
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.SegmentInterrupt)}},
			Required:  []string{"interrupts"},
			Forbidden: []string{"error", "detail"},
		}, PresenceRule{
			When:      []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.SegmentSuspended)}},
			Forbidden: []string{"interrupts", "error", "detail"},
		}),
	})

	// A pending set with no interrupts is not a thing to resume — it would leave
	// the client polling a run that will never move (contract §11.2).
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PendingInterruptSet](),
		Rules: []PresenceRule{{
			Required: []string{"rootRunId", "sessionId", "interrupts", "createdAt"},
		}},
	})

	// The archive's runs obey the same child-edge rule the live summaries do: all
	// three edges or none. A child additionally carries NO protocol profile — it
	// reads its root's, and a child with one of its own would import a run claiming
	// a contract nothing negotiated.
	//
	// The other half — a ROOT must carry one — is not here: "root" is the absence of
	// those edges, and absence is not something a PresenceRule can condition on. It
	// is an aggregate invariant of the import, checked where the archive is turned
	// into a session.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactRun](),
		Rules: append([]PresenceRule{{
			When:      []FieldCondition{{Field: "spawnedByItemId", Operator: OperatorPresent}},
			Forbidden: []string{"protocolProfile"},
		}}, childLineageRules()...),
	})

	// A failure outcome's explanation lives on `error`, and only there — the
	// archive's copy of the live rule, because an exported run has to say why it
	// failed as unambiguously as a live one.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactOutcome](),
		Rules:  failureArtifactRules(),
	})
}

func failureArtifactRules() []PresenceRule {
	return []PresenceRule{
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.ArtifactOutcomeTimedOut)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.ArtifactOutcomeFailed)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.ArtifactOutcomeLost)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
	}
}

// childLineageRules say the three child edges are all-or-none: a run either
// carries every one of them or is a root (§4.2). Stated as one rule per edge
// rather than "root forbids them", because presence is the only thing a
// PresenceRule can condition on and each edge is the condition for the other two.
//
// The contract's other half — that neither RunId equals the run's own id — is
// NOT here. JSON Schema cannot compare two fields, so it could not be one of the
// three equivalent statements §11.2 asks for; it is an identity invariant of the
// child-creation transaction. It is registered as a system invariant and proved
// by admission and durable-adapter fixtures; fusing an inequality into a
// presence rule would be one primitive doing two jobs.
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

// failureTerminalRules define the terminal outcomes whose typed problem is
// mandatory and mutually exclusive with the free-form detail field.
func failureTerminalRules() []PresenceRule {
	return []PresenceRule{
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeTimedOut)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeFailed)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
		{When: []FieldCondition{{Field: "type", Operator: OperatorEquals, Value: string(protocol.OutcomeLost)}}, Required: []string{"error"}, Forbidden: []string{"detail"}},
	}
}

func registerStateKeys(s *Shapes) {
	// `plan` is the only first-party shared-state key today.
	//
	// The event is authoritative and replayable in the Runtime-instance-local segment
	// window; the persisted projection is independently recoverable through
	// plan.get. These are three different guarantees and are kept explicit.
	s.stateKey(StateKeySpec{
		Key:            string(protocol.StatePlan),
		RecoveryMethod: "plan.get",
		Scope:          StateScopeSession,
		Writer:         StateWriterRootRun,
		Feature:        "plan",
		Stability:      stable,
		PayloadType:    typeOf[protocol.StateSnapshot](),
	})
}

func registerCarriedShapes(s *Shapes) {
	// `params._meta` is stripped before typed params are decoded, so the walk cannot
	// reach it — yet every client constructs it (API.md §2.4).
	s.carriedShape(CarriedSpec{Carrier: "params._meta", GoType: typeOf[protocol.RequestMeta]()})
}
