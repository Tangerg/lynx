package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// samplesDir holds the shared canonical wire samples. They live under the
// frontend tree (its tsconfig rootDir) so the TS `satisfies` test can import
// them directly; the Go side — the protocol SSOT — reads them cross-module.
// See app/desktop/docs/protocol/API.md §14 (machine-readable artifacts / drift
// gate) and app/desktop/docs/protocol/API.md.
const samplesDir = "../../../../desktop/frontend/src/rpc/samples"

// wireSamples pins each canonical sample to the protocol type it must
// round-trip through. It covers the whole API.md §4 data catalog + §5 streaming:
// every discriminated-union variant (StreamEvent / Item / ItemDelta / RunOutcome
// / Interrupt / WorkspaceEvent / DiffRow) plus the core shapes and method
// request/response envelopes. A few pins are deliberately asymmetric with the TS
// side (documented in API.md §14): resume reuses StartRunResponse; list methods
// return the generic Page[T].
var wireSamples = []struct {
	file   string
	target func() any
}{
	// §5 streaming — RunEvent envelope over every StreamEvent variant.
	{"segment.started.json", func() any { return new(RunEvent) }},
	{"segment.progress.json", func() any { return new(RunEvent) }},
	{"segment.finished.json", func() any { return new(RunEvent) }},
	{"item.started.json", func() any { return new(RunEvent) }},
	{"item.delta.json", func() any { return new(RunEvent) }},
	{"item.completed.json", func() any { return new(RunEvent) }},
	{"state.snapshot.json", func() any { return new(RunEvent) }},
	{"state.delta.json", func() any { return new(RunEvent) }},
	{"custom.json", func() any { return new(RunEvent) }},

	// §4.3 Item union (bare) + ContentBlock.
	{"item.userMessage.json", func() any { return new(Item) }},
	{"item.reasoning.json", func() any { return new(Item) }},
	{"item.plan.json", func() any { return new(Item) }},
	{"item.question.json", func() any { return new(Item) }},
	{"item.compaction.json", func() any { return new(Item) }},
	{"content.image.json", func() any { return new(ContentBlock) }},

	// §5.1 ItemDelta union (bare).
	{"delta.reasoning.json", func() any { return new(ItemDelta) }},
	{"delta.toolArguments.json", func() any { return new(ItemDelta) }},
	{"delta.toolOutput.json", func() any { return new(ItemDelta) }},
	{"delta.plan.json", func() any { return new(ItemDelta) }},

	// §4.2 Run — RunOutcome union, RunRef, Interrupt union, method envelopes.
	{"outcome.error.json", func() any { return new(RunOutcome) }},
	{"outcome.maxSteps.json", func() any { return new(RunOutcome) }},
	{"outcome.maxBudget.json", func() any { return new(RunOutcome) }},
	{"outcome.canceled.json", func() any { return new(RunOutcome) }},
	{"outcome.interrupt.json", func() any { return new(RunOutcome) }},
	{"runref.full.json", func() any { return new(RunRef) }},
	{"interrupt.approval.json", func() any { return new(Interrupt) }},
	{"interrupt.question.json", func() any { return new(Interrupt) }},
	{"interrupt.toolResult.json", func() any { return new(Interrupt) }},
	{"method.runs.start.req.json", func() any { return new(StartRunRequest) }},
	{"method.runs.start.resp.json", func() any { return new(StartRunResponse) }},
	{"method.runs.resume.req.json", func() any { return new(ResumeRunRequest) }},
	// Go has no ResumeRunResponse — ResumeRun returns *StartRunResponse; {runId}
	// round-trips identically (TS pins it as ResumeRunResponse).
	{"method.runs.resume.resp.json", func() any { return new(StartRunResponse) }},

	// §4.1 Session — Session/Project + method envelopes.
	{"session.json", func() any { return new(Session) }},
	{"project.json", func() any { return new(Project) }},
	{"method.sessions.create.req.json", func() any { return new(CreateSessionRequest) }},
	{"method.sessions.list.resp.json", func() any { return new(Page[Session]) }},
	{"method.sessions.rollback.req.json", func() any { return new(RollbackSessionRequest) }},
	{"method.sessions.rollback.resp.json", func() any { return new(RollbackSessionResponse) }},
	{"method.sessions.fork.req.json", func() any { return new(ForkSessionRequest) }},
	{"method.sessions.export.resp.json", func() any { return new(ExportSessionResponse) }},
	{"session.artifact.json", func() any { return new(SessionArtifact) }},

	// §4.5 Workspace — WorkspaceEvent union, Diff/DiffRow, file shapes, methods.
	{"wsevent.files-changed.json", func() any { return new(WorkspaceEvent) }},
	{"wsevent.skills-changed.json", func() any { return new(WorkspaceEvent) }},
	{"wsevent.mcp-serverChanged.json", func() any { return new(WorkspaceEvent) }},
	{"wsevent.schedules-fired.json", func() any { return new(WorkspaceEvent) }},
	{"wsevent.resync.json", func() any { return new(WorkspaceEvent) }},
	{"ws.diff.json", func() any { return new(Diff) }},
	{"ws.fileChange.json", func() any { return new(WorkspaceFileChange) }},
	{"ws.fileHead.json", func() any { return new(FileHead) }},
	{"ws.grepResult.json", func() any { return new(GrepResult) }},
	{"ws.searchHit.json", func() any { return new(SearchHit) }},
	{"ws.fileContent.json", func() any { return new(FileContent) }},
	{"method.getDiff.req.json", func() any { return new(GetDiffRequest) }},
	{"method.listFileChanges.req.json", func() any { return new(WorkspaceListQuery) }},
	{"method.listFileChanges.resp.json", func() any { return new(Page[WorkspaceFileChange]) }},
	{"method.grep.req.json", func() any { return new(GrepRequest) }},

	// §4.6 Approval + §4.9 providers/models/usage/codebase.
	{"approvalRule.json", func() any { return new(ApprovalRule) }},
	{"approvalMode.resp.json", func() any { return new(ApprovalModeResult) }},
	{"approvalRules.resp.json", func() any { return new(ListApprovalRulesResult) }},
	{"provider.json", func() any { return new(Provider) }},
	{"providers.list.resp.json", func() any { return new(Page[Provider]) }},
	{"model.json", func() any { return new(Model) }},
	{"models.list.resp.json", func() any { return new(Page[Model]) }},
	{"utilityRole.json", func() any { return new(UtilityRole) }},
	{"embeddingRole.json", func() any { return new(EmbeddingRole) }},
	{"usageSummary.json", func() any { return new(UsageSummary) }},
	{"codebaseStatus.json", func() any { return new(CodebaseStatus) }},
	{"codebaseHit.json", func() any { return new(CodebaseHit) }},
	{"codebaseSearch.resp.json", func() any { return new(CodebaseSearchResult) }},

	// §3/§9 discovery, request metadata + §4.10 config surfaces.
	{"method.discover.resp.json", func() any { return new(DiscoverResponse) }},
	{"request.meta.json", func() any { return new(RequestMeta) }},
	{"schedule.json", func() any { return new(Schedule) }},
	{"recipe.json", func() any { return new(Recipe) }},
	{"skill.json", func() any { return new(Skill) }},
	{"managedSkill.json", func() any { return new(ManagedSkill) }},
	{"skillDraft.json", func() any { return new(SkillDraft) }},
	{"agentDoc.json", func() any { return new(AgentDoc) }},
	{"mcpServer.json", func() any { return new(McpServer) }},
	{"mcpServerConfig.json", func() any { return new(McpServerConfig) }},
	{"hooksList.json", func() any { return new(HooksListResult) }},
	{"memoryEntry.json", func() any { return new(MemoryEntry) }},
	{"problemData.json", func() any { return new(ProblemData) }},
	{"feedback.req.json", func() any { return new(FeedbackRequest) }},
}

// TestWireGoldenRoundTrip is the Go half of the §14 drift gate: every canonical
// sample must unmarshal into the SSOT type and re-marshal to a SEMANTICALLY
// identical object. A Go struct that drops a field (unknown → discarded) or adds
// a non-omitempty zero diverges from the sample and fails here — catching the
// `items` vs `data` class of drift the moment the Go side moves. The TS side
// (frontend rpc/samples.test.ts) pins the SAME files against the hand-written
// wire types, so the two together pin one contract.
func TestWireGoldenRoundTrip(t *testing.T) {
	for _, s := range wireSamples {
		t.Run(s.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(samplesDir, s.file))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}

			target := s.target()
			if err := json.Unmarshal(raw, target); err != nil {
				t.Fatalf("unmarshal into %T: %v", target, err)
			}
			reencoded, err := json.Marshal(target)
			if err != nil {
				t.Fatalf("re-marshal %T: %v", target, err)
			}

			// Compare as generic maps: order-independent + semantic (a field the
			// Go type can't represent is dropped on re-marshal → the maps differ).
			var want, got map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("decode sample as map: %v", err)
			}
			if err := json.Unmarshal(reencoded, &got); err != nil {
				t.Fatalf("decode re-encoded as map: %v", err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Errorf("wire drift — sample and Go round-trip disagree\n sample:    %s\n re-marshal: %s", raw, reencoded)
			}
		})
	}
}
