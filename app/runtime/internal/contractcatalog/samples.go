// Package contractcatalog owns Runtime's private artifact-generation catalog.
// It binds public protocol values to generated schemas and samples without
// publishing reflection machinery as part of the Go client contract.
package contractcatalog

import (
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

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
// projects it into the manifest and the published TypeScript sample index.
// The SAMPLES stay hand-written — §11.3 forbids generating a fixture and then proving
// it with the same-source schema — and so does this binding.

// Sample binds one sample file to the published shape it must satisfy.
type Sample struct {
	File string
	Type reflect.Type
}

// Samples returns that binding, grouped the way the canonical docs
// group the catalog. Each artifact consumer receives its own slice so one
// generator or test cannot rewrite the catalog observed by another.
func Samples() []Sample {
	return []Sample{

		// §5 streaming — RunEvent envelope over every StreamEvent variant.
		{"segment.started.json", reflect.TypeFor[protocol.RunEvent]()},
		{"segment.progress.json", reflect.TypeFor[protocol.RunEvent]()},
		{"segment.finished.json", reflect.TypeFor[protocol.RunEvent]()},
		{"item.started.json", reflect.TypeFor[protocol.RunEvent]()},
		{"item.delta.json", reflect.TypeFor[protocol.RunEvent]()},
		{"item.completed.json", reflect.TypeFor[protocol.RunEvent]()},
		{"state.snapshot.json", reflect.TypeFor[protocol.RunEvent]()},

		// §4.3 Item union (bare) + ContentBlock.
		{"item.userMessage.json", reflect.TypeFor[protocol.Item]()},
		{"item.reasoning.json", reflect.TypeFor[protocol.Item]()},
		{"item.question.json", reflect.TypeFor[protocol.Item]()},
		{"item.compaction.json", reflect.TypeFor[protocol.Item]()},
		{"content.image.json", reflect.TypeFor[protocol.ContentBlock]()},

		// §5.1 ItemDelta union (bare).
		{"delta.reasoning.json", reflect.TypeFor[protocol.ItemDelta]()},
		{"delta.toolArguments.json", reflect.TypeFor[protocol.ItemDelta]()},
		{"delta.toolOutput.json", reflect.TypeFor[protocol.ItemDelta]()},

		// §4.2 Run — RunOutcome union, RunRef, Interrupt union, method envelopes.
		{"outcome.failed.json", reflect.TypeFor[protocol.RunOutcome]()},
		{"outcome.maxSteps.json", reflect.TypeFor[protocol.RunOutcome]()},
		{"outcome.maxBudget.json", reflect.TypeFor[protocol.RunOutcome]()},
		{"outcome.canceled.json", reflect.TypeFor[protocol.RunOutcome]()},
		// The two stops a run survives are SegmentOutcome-only, and bound to it: a
		// RunOutcome can never carry either. `suspended` is produced only for a root
		// profile that negotiated features.subagents.
		{"segment.outcome.interrupt.json", reflect.TypeFor[protocol.SegmentOutcome]()},
		{"segment.outcome.suspended.json", reflect.TypeFor[protocol.SegmentOutcome]()},
		{"runref.full.json", reflect.TypeFor[protocol.RunRef]()},
		// A summary travels on its own on the cold read, and a waiting run is the one
		// state with no outcome to explain it — the pair a full RunRef cannot show.
		{"runsummary.waiting.json", reflect.TypeFor[protocol.RunSummary]()},
		// The three child edges are all-or-none, and only a child carries them.
		{"runsummary.child.json", reflect.TypeFor[protocol.RunSummary]()},
		{"interrupt.approval.json", reflect.TypeFor[protocol.Interrupt]()},
		{"interrupt.question.json", reflect.TypeFor[protocol.Interrupt]()},
		{"method.runs.start.req.json", reflect.TypeFor[protocol.StartRunRequest]()},
		{"method.runs.start.resp.json", reflect.TypeFor[protocol.StartRunResponse]()},
		{"method.runs.resume.req.json", reflect.TypeFor[protocol.ResumeRunRequest]()},
		{"method.runs.resume.resp.json", reflect.TypeFor[protocol.ResumeRunResponse]()},
		// Subscribe has its own pair because it is NOT a run-opening ack: the request
		// must name a segment, and the response carries a stream position instead of a
		// user item. The sample's headEventId is deliberately an opaque token — it is
		// there to be stored and handed back, and a fixture that spelled out a
		// sequence would invite a client to read one.
		{"method.runs.subscribe.req.json", reflect.TypeFor[protocol.SubscribeRunRequest]()},
		{"method.runs.subscribe.resp.json", reflect.TypeFor[protocol.SubscribeRunResponse]()},

		// §4.1 Session — Session/WorkspaceSummary + method envelopes.
		{"session.json", reflect.TypeFor[protocol.Session]()},
		{"workspace.json", reflect.TypeFor[protocol.WorkspaceSummary]()},
		{"method.sessions.create.req.json", reflect.TypeFor[protocol.CreateSessionRequest]()},
		{"method.sessions.list.resp.json", reflect.TypeFor[protocol.Page[protocol.Session]]()},
		{"method.sessions.rollback.req.json", reflect.TypeFor[protocol.RollbackSessionRequest]()},
		{"method.sessions.rollback.resp.json", reflect.TypeFor[protocol.RollbackSessionResponse]()},
		{"method.sessions.fork.req.json", reflect.TypeFor[protocol.ForkSessionRequest]()},
		{"method.sessions.export.resp.json", reflect.TypeFor[protocol.ExportSessionResponse]()},
		{"session.artifact.json", reflect.TypeFor[protocol.SessionArtifact]()},

		// §7.3 RuntimeEvent union — one change signal per topic, plus the frame that
		// says the stream lost its place.
		{"rtevent.files-changed.json", reflect.TypeFor[protocol.RuntimeEvent]()},
		{"rtevent.skills-changed.json", reflect.TypeFor[protocol.RuntimeEvent]()},
		{"rtevent.mcp-changed.json", reflect.TypeFor[protocol.RuntimeEvent]()},
		{"rtevent.schedules-changed.json", reflect.TypeFor[protocol.RuntimeEvent]()},
		{"rtevent.state-changed.json", reflect.TypeFor[protocol.RuntimeEvent]()},
		{"rtevent.resync.json", reflect.TypeFor[protocol.RuntimeEvent]()},

		// §4.5 Workspace — Diff/DiffRow, file shapes, methods.
		{"ws.diff.json", reflect.TypeFor[protocol.Diff]()},
		{"ws.fileChange.json", reflect.TypeFor[protocol.WorkspaceFileChange]()},
		{"ws.fileHead.json", reflect.TypeFor[protocol.FileHead]()},
		{"ws.grepResult.json", reflect.TypeFor[protocol.GrepResult]()},
		{"ws.fileContent.json", reflect.TypeFor[protocol.FileContent]()},
		{"method.getDiff.req.json", reflect.TypeFor[protocol.GetDiffRequest]()},
		{"method.listFileChanges.req.json", reflect.TypeFor[protocol.WorkspaceQuery]()},
		{"method.listFileChanges.resp.json", reflect.TypeFor[protocol.Page[protocol.WorkspaceFileChange]]()},
		{"method.grep.req.json", reflect.TypeFor[protocol.GrepRequest]()},

		// §4.6 Approval + §4.9 providers/models/usage/codebase.
		{"approvalRule.json", reflect.TypeFor[protocol.ApprovalRule]()},
		{"approvalMode.resp.json", reflect.TypeFor[protocol.ApprovalModeResult]()},
		{"approvalRules.resp.json", reflect.TypeFor[protocol.ListApprovalRulesResult]()},
		{"provider.json", reflect.TypeFor[protocol.Provider]()},
		{"providers.list.resp.json", reflect.TypeFor[protocol.Page[protocol.Provider]]()},
		{"model.json", reflect.TypeFor[protocol.Model]()},
		{"models.list.resp.json", reflect.TypeFor[protocol.Page[protocol.Model]]()},
		{"utilityRole.json", reflect.TypeFor[protocol.UtilityRole]()},
		{"embeddingRole.json", reflect.TypeFor[protocol.EmbeddingRole]()},
		{"usageSummary.json", reflect.TypeFor[protocol.UsageSummary]()},
		{"codebaseStatus.json", reflect.TypeFor[protocol.CodebaseStatus]()},
		{"codebaseHit.json", reflect.TypeFor[protocol.CodebaseHit]()},
		{"codebaseSearch.resp.json", reflect.TypeFor[protocol.CodebaseSearchResult]()},

		// §3/§9 discovery, request metadata + §4.10 config surfaces.
		{"method.discover.resp.json", reflect.TypeFor[protocol.DiscoverResponse]()},
		{"request.meta.json", reflect.TypeFor[protocol.RequestMeta]()},
		{"schedule.json", reflect.TypeFor[protocol.Schedule]()},
		{"recipe.json", reflect.TypeFor[protocol.Recipe]()},
		{"skill.json", reflect.TypeFor[protocol.Skill]()},
		{"managedSkill.json", reflect.TypeFor[protocol.ManagedSkill]()},
		{"skillProposal.json", reflect.TypeFor[protocol.SkillProposal]()},
		{"agentDoc.json", reflect.TypeFor[protocol.AgentDoc]()},
		{"mcpAuthorizationAttempt.json", reflect.TypeFor[protocol.MCPAuthorizationAttempt]()},
		{"mcpServer.json", reflect.TypeFor[protocol.MCPServer]()},
		{"hooksList.json", reflect.TypeFor[protocol.HooksListResult]()},
		{"knowledgeEntry.json", reflect.TypeFor[protocol.KnowledgeEntry]()},
		{"agentMemoryItem.json", reflect.TypeFor[protocol.AgentMemoryItem]()},
		{"goal.json", reflect.TypeFor[protocol.Goal]()},
		{"problemData.json", reflect.TypeFor[protocol.ProblemData]()},
		{"feedback.req.json", reflect.TypeFor[protocol.FeedbackRequest]()},
	}
}
