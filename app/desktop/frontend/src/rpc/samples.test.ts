import { describe, expect, it } from "vitest";

import type {
  AgentDoc,
  ApprovalMode,
  ApprovalRule,
  CodebaseHit,
  CodebaseStatus,
  ContentBlock,
  CreateSessionRequest,
  Diff,
  EmbeddingRole,
  ExportSessionResponse,
  FeedbackRequest,
  FileContent,
  FileHead,
  ForkSessionRequest,
  GrepResult,
  HooksListResult,
  DiscoverResponse,
  Interrupt,
  Item,
  ItemDelta,
  McpServer,
  McpServerConfig,
  MemoryEntry,
  AgentMemoryItem,
  Goal,
  Model,
  Page,
  ProblemData,
  Project,
  Provider,
  Recipe,
  RequestMeta,
  ResumeRunRequest,
  ResumeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  RunOutcome,
  RunRef,
  Schedule,
  SearchHit,
  Session,
  SessionArtifact,
  Skill,
  ManagedSkill,
  SkillDraft,
  StartRunRequest,
  StartRunResponse,
  UsageSummary,
  UtilityRole,
  WorkspaceEvent,
  WorkspaceFileChange,
} from "./shapes";

import agentDoc from "./samples/agentDoc.json";
import approvalModeResp from "./samples/approvalMode.resp.json";
import approvalRule from "./samples/approvalRule.json";
import approvalRulesResp from "./samples/approvalRules.resp.json";
import codebaseHit from "./samples/codebaseHit.json";
import codebaseSearchResp from "./samples/codebaseSearch.resp.json";
import codebaseStatus from "./samples/codebaseStatus.json";
import contentImage from "./samples/content.image.json";
import custom from "./samples/custom.json";
import deltaPlan from "./samples/delta.plan.json";
import deltaReasoning from "./samples/delta.reasoning.json";
import deltaToolArguments from "./samples/delta.toolArguments.json";
import deltaToolOutput from "./samples/delta.toolOutput.json";
import embeddingRole from "./samples/embeddingRole.json";
import feedbackReq from "./samples/feedback.req.json";
import hooksList from "./samples/hooksList.json";
import interruptApproval from "./samples/interrupt.approval.json";
import interruptQuestion from "./samples/interrupt.question.json";
import interruptToolResult from "./samples/interrupt.toolResult.json";
import itemCompaction from "./samples/item.compaction.json";
import itemCompleted from "./samples/item.completed.json";
import itemDelta from "./samples/item.delta.json";
import itemPlan from "./samples/item.plan.json";
import itemQuestion from "./samples/item.question.json";
import itemReasoning from "./samples/item.reasoning.json";
import itemStarted from "./samples/item.started.json";
import itemUserMessage from "./samples/item.userMessage.json";
import mcpServer from "./samples/mcpServer.json";
import mcpServerConfig from "./samples/mcpServerConfig.json";
import memoryEntry from "./samples/memoryEntry.json";
import agentMemoryItem from "./samples/agentMemoryItem.json";
import goal from "./samples/goal.json";
import getDiffReq from "./samples/method.getDiff.req.json";
import grepReq from "./samples/method.grep.req.json";
import discoverResp from "./samples/method.discover.resp.json";
import listFileChangesReq from "./samples/method.listFileChanges.req.json";
import listFileChangesResp from "./samples/method.listFileChanges.resp.json";
import runsResumeReq from "./samples/method.runs.resume.req.json";
import runsResumeResp from "./samples/method.runs.resume.resp.json";
import runsStartReq from "./samples/method.runs.start.req.json";
import runsStartResp from "./samples/method.runs.start.resp.json";
import sessionsCreateReq from "./samples/method.sessions.create.req.json";
import sessionsExportResp from "./samples/method.sessions.export.resp.json";
import sessionsForkReq from "./samples/method.sessions.fork.req.json";
import sessionsListResp from "./samples/method.sessions.list.resp.json";
import sessionsRollbackReq from "./samples/method.sessions.rollback.req.json";
import sessionsRollbackResp from "./samples/method.sessions.rollback.resp.json";
import model from "./samples/model.json";
import modelsListResp from "./samples/models.list.resp.json";
import outcomeCanceled from "./samples/outcome.canceled.json";
import outcomeError from "./samples/outcome.error.json";
import outcomeInterrupt from "./samples/outcome.interrupt.json";
import outcomeMaxBudget from "./samples/outcome.maxBudget.json";
import outcomeMaxSteps from "./samples/outcome.maxSteps.json";
import problemData from "./samples/problemData.json";
import project from "./samples/project.json";
import provider from "./samples/provider.json";
import providersListResp from "./samples/providers.list.resp.json";
import recipe from "./samples/recipe.json";
import requestMeta from "./samples/request.meta.json";
import runFinished from "./samples/segment.finished.json";
import runProgress from "./samples/segment.progress.json";
import runStarted from "./samples/segment.started.json";
import runrefFull from "./samples/runref.full.json";
import schedule from "./samples/schedule.json";
import sessionArtifact from "./samples/session.artifact.json";
import session from "./samples/session.json";
import skill from "./samples/skill.json";
import managedSkill from "./samples/managedSkill.json";
import skillDraft from "./samples/skillDraft.json";
import stateDelta from "./samples/state.delta.json";
import stateSnapshot from "./samples/state.snapshot.json";
import usageSummary from "./samples/usageSummary.json";
import utilityRole from "./samples/utilityRole.json";
import wsDiff from "./samples/ws.diff.json";
import wsFileChange from "./samples/ws.fileChange.json";
import wsFileContent from "./samples/ws.fileContent.json";
import wsFileHead from "./samples/ws.fileHead.json";
import wsGrepResult from "./samples/ws.grepResult.json";
import wsSearchHit from "./samples/ws.searchHit.json";
import wseventFilesChanged from "./samples/wsevent.files-changed.json";
import wseventMcpServerChanged from "./samples/wsevent.mcp-serverChanged.json";
import wseventResync from "./samples/wsevent.resync.json";
import wseventSchedulesFired from "./samples/wsevent.schedules-fired.json";
import wseventSkillsChanged from "./samples/wsevent.skills-changed.json";

// Strip the phantom id brands and widen literals to primitives, mirroring how
// `resolveJsonModule` types an imported JSON sample. RunId / SessionId / ItemId
// are `string & {brand}` (ids.ts) — a compile-time-only nominal tag absent on
// the wire — so a plain-string sample can be checked against the branded type
// without asserting the brand. Widening string-literal unions to `string` and
// boolean literals (e.g. `binary?: true`) to `boolean` matches JSON's own
// widening AND keeps the gate STRUCTURAL (field names + shapes), not a value
// check — discriminator/flag values are pinned by the Go round-trip and enforced
// at runtime by the fold layer.
type Unbrand<T> = T extends string
  ? string
  : T extends boolean
    ? boolean
    : T extends readonly (infer E)[]
      ? Unbrand<E>[]
      : T extends object
        ? { [K in keyof T]: Unbrand<T[K]> }
        : T;

// The TS half of the API.md §14 drift gate. wire<T>(sample) pins ONE canonical
// JSON sample to ONE §4/§5 wire type: the sample must structurally satisfy the
// hand-written type (brands stripped), so renaming / removing / retyping a
// shapes.ts field away from a sample fails `tsc` (the frontend `check` gate).
// The Go side (protocol/wire_golden_test.go) round-trips the SAME files against
// the SSOT structs, so the two pin one contract — replacing "keep in sync by
// review" with a machine check. Returns the sample so callers collect it for a
// runtime non-empty smoke.
function wire<T>(sample: Unbrand<T>): unknown {
  return sample;
}

const samples: unknown[] = [
  // §5 streaming — the RunEvent envelope over every StreamEvent variant.
  wire<RunEvent>(runStarted),
  wire<RunEvent>(runProgress),
  wire<RunEvent>(runFinished),
  wire<RunEvent>(itemStarted),
  wire<RunEvent>(itemDelta),
  wire<RunEvent>(itemCompleted),
  wire<RunEvent>(stateSnapshot),
  wire<RunEvent>(stateDelta),
  wire<RunEvent>(custom),

  // §4.3 Item union (bare) + ContentBlock.
  wire<Item>(itemUserMessage),
  wire<Item>(itemReasoning),
  wire<Item>(itemPlan),
  wire<Item>(itemQuestion),
  wire<Item>(itemCompaction),
  wire<ContentBlock>(contentImage),

  // §5.1 ItemDelta union (bare).
  wire<ItemDelta>(deltaReasoning),
  wire<ItemDelta>(deltaToolArguments),
  wire<ItemDelta>(deltaToolOutput),
  wire<ItemDelta>(deltaPlan),

  // §4.2 Run — RunOutcome union, RunRef, Interrupt union, method envelopes.
  wire<RunOutcome>(outcomeError),
  wire<RunOutcome>(outcomeMaxSteps),
  wire<RunOutcome>(outcomeMaxBudget),
  wire<RunOutcome>(outcomeCanceled),
  wire<RunOutcome>(outcomeInterrupt),
  wire<RunRef>(runrefFull),
  wire<Interrupt>(interruptApproval),
  wire<Interrupt>(interruptQuestion),
  wire<Interrupt>(interruptToolResult),
  wire<StartRunRequest>(runsStartReq),
  wire<StartRunResponse>(runsStartResp),
  wire<ResumeRunRequest>(runsResumeReq),
  wire<ResumeRunResponse>(runsResumeResp),

  // §4.1 Session — Session/Project + method envelopes.
  wire<Session>(session),
  wire<Project>(project),
  wire<CreateSessionRequest>(sessionsCreateReq),
  wire<Page<Session>>(sessionsListResp),
  wire<RollbackSessionRequest>(sessionsRollbackReq),
  wire<RollbackSessionResponse>(sessionsRollbackResp),
  wire<ForkSessionRequest>(sessionsForkReq),
  wire<ExportSessionResponse>(sessionsExportResp),
  wire<SessionArtifact>(sessionArtifact),

  // §4.5 Workspace — WorkspaceEvent union, Diff, file shapes, methods.
  wire<WorkspaceEvent>(wseventFilesChanged),
  wire<WorkspaceEvent>(wseventSkillsChanged),
  wire<WorkspaceEvent>(wseventMcpServerChanged),
  wire<WorkspaceEvent>(wseventSchedulesFired),
  wire<WorkspaceEvent>(wseventResync),
  wire<Diff>(wsDiff),
  wire<WorkspaceFileChange>(wsFileChange),
  wire<FileHead>(wsFileHead),
  wire<GrepResult>(wsGrepResult),
  wire<SearchHit>(wsSearchHit),
  wire<FileContent>(wsFileContent),
  // getDiff/grep/listFileChanges params are inline objects in methods.ts (no
  // named interface); pinned against those inline shapes.
  wire<{
    cwd?: string;
    path?: string;
    mode?: "worktree" | "base";
    format?: "rows" | "raw";
    limit?: number;
  }>(getDiffReq),
  wire<{ cwd?: string }>(listFileChangesReq),
  wire<Page<WorkspaceFileChange>>(listFileChangesResp),
  wire<{ query: string; cwd?: string; path?: string; limit?: number }>(grepReq),

  // §4.6 Approval + §4.9 providers/models/usage/codebase.
  wire<ApprovalRule>(approvalRule),
  wire<{ mode: ApprovalMode }>(approvalModeResp),
  wire<{ rules: ApprovalRule[] }>(approvalRulesResp),
  wire<Provider>(provider),
  wire<Page<Provider>>(providersListResp),
  wire<Model>(model),
  wire<Page<Model>>(modelsListResp),
  wire<UtilityRole>(utilityRole),
  wire<EmbeddingRole>(embeddingRole),
  wire<UsageSummary>(usageSummary),
  wire<CodebaseStatus>(codebaseStatus),
  wire<CodebaseHit>(codebaseHit),
  wire<{ hits: CodebaseHit[] }>(codebaseSearchResp),

  // §3/§9 discovery, request metadata + §4.10 config surfaces.
  wire<DiscoverResponse>(discoverResp),
  wire<RequestMeta>(requestMeta),
  wire<Schedule>(schedule),
  wire<Recipe>(recipe),
  wire<Skill>(skill),
  wire<ManagedSkill>(managedSkill),
  wire<SkillDraft>(skillDraft),
  wire<AgentDoc>(agentDoc),
  wire<McpServer>(mcpServer),
  wire<McpServerConfig>(mcpServerConfig),
  wire<HooksListResult>(hooksList),
  wire<MemoryEntry>(memoryEntry),
  wire<AgentMemoryItem>(agentMemoryItem),
  wire<Goal>(goal),
  wire<ProblemData>(problemData),
  wire<FeedbackRequest>(feedbackReq),
];

describe("wire golden samples", () => {
  it("every canonical sample parses to a non-empty object", () => {
    expect(samples.length).toBeGreaterThan(0);
    for (const s of samples) expect(s).toBeTruthy();
  });
});
