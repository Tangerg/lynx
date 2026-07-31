package protocol

import "reflect"

// The canonical wire samples and the shape each one must be.
//
// Contract §11.3 asks that the Go side, the TypeScript validator and the JSON Schema
// each check the SAME batch of hand-written fixtures — and "the same batch" only
// means something if one statement says which file is which shape. It was written
// twice: this table, and a parallel list of `wire<T>(sample)` pins on the TypeScript
// side. Two lists means two answers to "what is method.sessions.rollback.resp.json",
// and the one that drifts is silently checked against the wrong thing.
//
// It lives in non-test code because the artifact pipeline reads it: the generator
// projects it into the manifest and into the index the frontend's own check consumes.
// The SAMPLES stay hand-written — §11.3 forbids generating a fixture and then proving
// it with the same-source schema — and so does this binding.

// CanonicalSample binds one sample file to the published shape it must satisfy.
type CanonicalSample struct {
	File string
	Type reflect.Type
}

// CanonicalSamples is that binding, grouped the way the canonical docs group the
// catalog.
var CanonicalSamples = []CanonicalSample{

	// §5 streaming — RunEvent envelope over every StreamEvent variant.
	{"segment.started.json", reflect.TypeFor[RunEvent]()},
	{"segment.progress.json", reflect.TypeFor[RunEvent]()},
	{"segment.finished.json", reflect.TypeFor[RunEvent]()},
	{"item.started.json", reflect.TypeFor[RunEvent]()},
	{"item.delta.json", reflect.TypeFor[RunEvent]()},
	{"item.completed.json", reflect.TypeFor[RunEvent]()},
	{"state.snapshot.json", reflect.TypeFor[RunEvent]()},
	{"custom.json", reflect.TypeFor[RunEvent]()},

	// §4.3 Item union (bare) + ContentBlock.
	{"item.userMessage.json", reflect.TypeFor[Item]()},
	{"item.reasoning.json", reflect.TypeFor[Item]()},
	{"item.plan.json", reflect.TypeFor[Item]()},
	{"item.question.json", reflect.TypeFor[Item]()},
	{"item.compaction.json", reflect.TypeFor[Item]()},
	{"content.image.json", reflect.TypeFor[ContentBlock]()},

	// §5.1 ItemDelta union (bare).
	{"delta.reasoning.json", reflect.TypeFor[ItemDelta]()},
	{"delta.toolArguments.json", reflect.TypeFor[ItemDelta]()},
	{"delta.toolOutput.json", reflect.TypeFor[ItemDelta]()},
	{"delta.plan.json", reflect.TypeFor[ItemDelta]()},

	// §4.2 Run — RunOutcome union, RunRef, Interrupt union, method envelopes.
	{"outcome.error.json", reflect.TypeFor[RunOutcome]()},
	{"outcome.maxSteps.json", reflect.TypeFor[RunOutcome]()},
	{"outcome.maxBudget.json", reflect.TypeFor[RunOutcome]()},
	{"outcome.canceled.json", reflect.TypeFor[RunOutcome]()},
	// The two stops a run survives are SegmentOutcome-only, and bound to it: a
	// RunOutcome can never carry either. `suspended` is produced only for a root
	// profile that negotiated features.subagents.
	{"segment.outcome.interrupt.json", reflect.TypeFor[SegmentOutcome]()},
	{"segment.outcome.suspended.json", reflect.TypeFor[SegmentOutcome]()},
	{"runref.full.json", reflect.TypeFor[RunRef]()},
	// A summary travels on its own on the cold read, and a waiting run is the one
	// state with no outcome to explain it — the pair a full RunRef cannot show.
	{"runsummary.waiting.json", reflect.TypeFor[RunSummary]()},
	// The three child edges are all-or-none, and only a child carries them.
	{"runsummary.child.json", reflect.TypeFor[RunSummary]()},
	{"interrupt.approval.json", reflect.TypeFor[Interrupt]()},
	{"interrupt.question.json", reflect.TypeFor[Interrupt]()},
	{"interrupt.toolResult.json", reflect.TypeFor[Interrupt]()},
	{"method.runs.start.req.json", reflect.TypeFor[StartRunRequest]()},
	{"method.runs.start.resp.json", reflect.TypeFor[StartRunResponse]()},
	{"method.runs.resume.req.json", reflect.TypeFor[ResumeRunRequest]()},
	{"method.runs.resume.resp.json", reflect.TypeFor[ResumeRunResponse]()},
	// Subscribe has its own pair because it is NOT a run-opening ack: the request
	// must name a segment, and the response carries a stream position instead of a
	// user item. The sample's headEventId is deliberately an opaque token — it is
	// there to be stored and handed back, and a fixture that spelled out a
	// sequence would invite a client to read one.
	{"method.runs.subscribe.req.json", reflect.TypeFor[SubscribeRunRequest]()},
	{"method.runs.subscribe.resp.json", reflect.TypeFor[SubscribeRunResponse]()},

	// §4.1 Session — Session/Project + method envelopes.
	{"session.json", reflect.TypeFor[Session]()},
	{"project.json", reflect.TypeFor[Project]()},
	{"method.sessions.create.req.json", reflect.TypeFor[CreateSessionRequest]()},
	{"method.sessions.list.resp.json", reflect.TypeFor[Page[Session]]()},
	{"method.sessions.rollback.req.json", reflect.TypeFor[RollbackSessionRequest]()},
	{"method.sessions.rollback.resp.json", reflect.TypeFor[RollbackSessionResponse]()},
	{"method.sessions.fork.req.json", reflect.TypeFor[ForkSessionRequest]()},
	{"method.sessions.export.resp.json", reflect.TypeFor[ExportSessionResponse]()},
	{"session.artifact.json", reflect.TypeFor[SessionArtifact]()},

	// §7.3 RuntimeEvent union — one change signal per topic, plus the frame that
	// says the stream lost its place.
	{"rtevent.files-changed.json", reflect.TypeFor[RuntimeEvent]()},
	{"rtevent.skills-changed.json", reflect.TypeFor[RuntimeEvent]()},
	{"rtevent.mcp-changed.json", reflect.TypeFor[RuntimeEvent]()},
	{"rtevent.schedules-changed.json", reflect.TypeFor[RuntimeEvent]()},
	{"rtevent.state-changed.json", reflect.TypeFor[RuntimeEvent]()},
	{"rtevent.resync.json", reflect.TypeFor[RuntimeEvent]()},

	// §4.5 Workspace — Diff/DiffRow, file shapes, methods.
	{"ws.diff.json", reflect.TypeFor[Diff]()},
	{"ws.fileChange.json", reflect.TypeFor[WorkspaceFileChange]()},
	{"ws.fileHead.json", reflect.TypeFor[FileHead]()},
	{"ws.grepResult.json", reflect.TypeFor[GrepResult]()},
	{"ws.searchHit.json", reflect.TypeFor[SearchHit]()},
	{"ws.fileContent.json", reflect.TypeFor[FileContent]()},
	{"method.getDiff.req.json", reflect.TypeFor[GetDiffRequest]()},
	{"method.listFileChanges.req.json", reflect.TypeFor[WorkspaceListQuery]()},
	{"method.listFileChanges.resp.json", reflect.TypeFor[Page[WorkspaceFileChange]]()},
	{"method.grep.req.json", reflect.TypeFor[GrepRequest]()},

	// §4.6 Approval + §4.9 providers/models/usage/codebase.
	{"approvalRule.json", reflect.TypeFor[ApprovalRule]()},
	{"approvalMode.resp.json", reflect.TypeFor[ApprovalModeResult]()},
	{"approvalRules.resp.json", reflect.TypeFor[ListApprovalRulesResult]()},
	{"provider.json", reflect.TypeFor[Provider]()},
	{"providers.list.resp.json", reflect.TypeFor[Page[Provider]]()},
	{"model.json", reflect.TypeFor[Model]()},
	{"models.list.resp.json", reflect.TypeFor[Page[Model]]()},
	{"utilityRole.json", reflect.TypeFor[UtilityRole]()},
	{"embeddingRole.json", reflect.TypeFor[EmbeddingRole]()},
	{"usageSummary.json", reflect.TypeFor[UsageSummary]()},
	{"codebaseStatus.json", reflect.TypeFor[CodebaseStatus]()},
	{"codebaseHit.json", reflect.TypeFor[CodebaseHit]()},
	{"codebaseSearch.resp.json", reflect.TypeFor[CodebaseSearchResult]()},

	// §3/§9 discovery, request metadata + §4.10 config surfaces.
	{"method.discover.resp.json", reflect.TypeFor[DiscoverResponse]()},
	{"request.meta.json", reflect.TypeFor[RequestMeta]()},
	{"schedule.json", reflect.TypeFor[Schedule]()},
	{"recipe.json", reflect.TypeFor[Recipe]()},
	{"skill.json", reflect.TypeFor[Skill]()},
	{"managedSkill.json", reflect.TypeFor[ManagedSkill]()},
	{"skillDraft.json", reflect.TypeFor[SkillDraft]()},
	{"agentDoc.json", reflect.TypeFor[AgentDoc]()},
	{"mcpServer.json", reflect.TypeFor[McpServer]()},
	{"mcpServerConfig.json", reflect.TypeFor[McpServerConfig]()},
	{"hooksList.json", reflect.TypeFor[HooksListResult]()},
	{"memoryEntry.json", reflect.TypeFor[MemoryEntry]()},
	{"agentMemoryItem.json", reflect.TypeFor[AgentMemoryItem]()},
	{"goal.json", reflect.TypeFor[Goal]()},
	{"problemData.json", reflect.TypeFor[ProblemData]()},
	{"feedback.req.json", reflect.TypeFor[FeedbackRequest]()},
}
