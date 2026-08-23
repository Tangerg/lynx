import {
  LyraClient,
  protocolVersion,
  type DiscoverResponse,
  type CancelRunResponse,
  type CodebaseReindexResponse,
  type CodebaseSearchResult,
  type CodebaseStatus,
  type ContentBlock,
  type CreateSessionRequest,
	type CreateScheduleRequest,
  type Diff,
  type EmptyObject,
  type FileContent,
  type FileEntry,
	type FeedbackRequest,
	type ExportSessionResponse,
	type ForkSessionRequest,
  type AgentDoc,
	type ApprovalModeResult,
	type ApprovalRule,
  type Goal,
  type GoalBudget,
	type HooksListResult,
  type GrepResult,
  type InterruptResponse,
  type AgentMemoryItem,
  type AgentMemoryList,
  type AgentMemoryReviewDecision,
  type AgentMemoryScope,
  type KnowledgeEntry,
  type KnowledgeScope,
	type ListItemsRequest,
	type ListItemsResponse,
  type ManagedSkill,
  type MCPAuthorizationAttempt,
  type MCPServer,
  type MCPServerCandidate,
  type MCPTestResult,
  type MCPTool,
  type Model,
	type EmbeddingRole,
	  type Page,
	type Provider,
	type ProviderTestResult,
  type RequestMeta,
  type Recipe,
  type ResumeRunRequest,
  type ResumeRunResponse,
  type RuntimeEvent,
  type RuntimeConnection,
	type RuntimeTopic,
  type OpenRuntimeStream,
  type RunEvent,
	type RollbackSessionRequest,
	type RollbackSessionResponse,
	type RunScheduleNowResponse,
	type Schedule,
  type Session,
	type SessionArtifact,
  type SessionSnapshot,
  type Skill,
  type SkillProposal,
  type SkillProposalRef,
  type StartRunResponse,
  type ToolSpec,
	type UtilityRole,
	type Usage,
	type UsageSummary,
	type UsageSummaryRequest,
	type UpdateProviderRequest,
  type UpdateMCPServerRequest,
  type UpdateSessionRequest,
	type UpdateScheduleRequest,
  type WorkspaceRef,
  type WorkspaceFileChange,
} from "@lyra/runtime-contract";

const clientMeta: RequestMeta = {
  protocolVersion,
  clientInfo: { name: "lyra-desktop-app2", version: "0.0.0" },
  clientCapabilities: {
    features: {
      plan: { enabled: true },
      goals: { enabled: true },
      subagents: { enabled: true },
    },
    interruptTypes: ["approval", "question"],
  },
};

export const runtimeQueryKeys = {
  scope(connection: RuntimeConnection) {
    return [
      "runtime",
      connection.instanceId,
      connection.generation,
    ] as const;
  },
  sessions(connection: RuntimeConnection) {
    return [...this.scope(connection), "sessions"] as const;
  },
	usageSummary(connection: RuntimeConnection, sinceDays: number) {
		return [...this.scope(connection), "usage", "summary", sinceDays] as const;
	},
	sessionUsage(connection: RuntimeConnection, sessionId: string) {
		return [...this.scope(connection), "usage", "session", sessionId] as const;
	},
	sessionHistory(connection: RuntimeConnection, sessionId: string) {
		return [...this.scope(connection), "session", sessionId, "history"] as const;
	},
  snapshot(connection: RuntimeConnection, sessionId: string) {
    return [...this.scope(connection), "session", sessionId, "snapshot"] as const;
  },
  models(connection: RuntimeConnection, provider: string) {
    return [...this.scope(connection), "models", provider] as const;
  },
	providers(connection: RuntimeConnection) {
		return [...this.scope(connection), "providers"] as const;
	},
	modelRole(connection: RuntimeConnection, role: "utility" | "embedding") {
		return [...this.scope(connection), "model-role", role] as const;
	},
  mcp(connection: RuntimeConnection) {
    return [...this.scope(connection), "mcp"] as const;
  },
  mcpServers(connection: RuntimeConnection) {
    return [...this.mcp(connection), "servers"] as const;
  },
  mcpTools(connection: RuntimeConnection, server: string) {
    return [...this.mcp(connection), "tools", server] as const;
  },
	approvals(connection: RuntimeConnection) {
		return [...this.scope(connection), "approvals"] as const;
	},
	approvalMode(connection: RuntimeConnection) {
		return [...this.approvals(connection), "mode"] as const;
	},
	approvalRules(connection: RuntimeConnection, sessionId: string) {
		return [...this.approvals(connection), "rules", sessionId] as const;
	},
	hooks(connection: RuntimeConnection) {
		return [...this.scope(connection), "hooks"] as const;
	},
	workspaceHooks(connection: RuntimeConnection, workspacePath: string) {
		return [...this.hooks(connection), workspacePath] as const;
	},
	schedules(connection: RuntimeConnection) {
		return [...this.scope(connection), "schedules"] as const;
	},
  workspace(connection: RuntimeConnection, path: string) {
    return [...this.scope(connection), "workspace", path] as const;
  },
  workspaceFiles(
    connection: RuntimeConnection,
    workspacePath: string,
    directory: string,
  ) {
    return [
      ...this.workspace(connection, workspacePath),
      "files",
      directory,
    ] as const;
  },
  workspaceFile(
    connection: RuntimeConnection,
    workspacePath: string,
    path: string,
    startLine: number,
  ) {
    return [
      ...this.workspace(connection, workspacePath),
      "file",
      path,
      startLine,
    ] as const;
  },
  workspaceSearch(
    connection: RuntimeConnection,
    workspacePath: string,
    query: string,
  ) {
    return [
      ...this.workspace(connection, workspacePath),
      "search",
      query,
    ] as const;
  },
  workspaceChanges(connection: RuntimeConnection, workspacePath: string) {
    return [...this.workspace(connection, workspacePath), "changes"] as const;
  },
  workspaceRecipes(connection: RuntimeConnection, workspacePath: string) {
    return [...this.workspace(connection, workspacePath), "recipes"] as const;
  },
  workspaceAgentDocs(connection: RuntimeConnection, workspacePath: string) {
    return [...this.workspace(connection, workspacePath), "agent-docs"] as const;
  },
  workspaceDiff(
    connection: RuntimeConnection,
    workspacePath: string,
    path: string,
    mode: "worktree" | "base",
  ) {
    return [
      ...this.workspace(connection, workspacePath),
      "diff",
      mode,
      path,
    ] as const;
  },
  codebase(connection: RuntimeConnection, workspacePath: string) {
    return [...this.scope(connection), "codebase", workspacePath] as const;
  },
  codebaseStatus(connection: RuntimeConnection, workspacePath: string) {
    return [...this.codebase(connection, workspacePath), "status"] as const;
  },
  codebaseSearch(
    connection: RuntimeConnection,
    workspacePath: string,
    query: string,
  ) {
    return [
      ...this.codebase(connection, workspacePath),
      "search",
      query,
    ] as const;
  },
  skills(connection: RuntimeConnection) {
    return [...this.scope(connection), "skills"] as const;
  },
  discoveredSkills(connection: RuntimeConnection, workspacePath: string) {
    return [...this.skills(connection), "discovered", workspacePath] as const;
  },
  skillProposals(connection: RuntimeConnection, workspacePath: string) {
    return [...this.skills(connection), "proposals", workspacePath] as const;
  },
  skillLibrary(connection: RuntimeConnection) {
    return [...this.skills(connection), "library"] as const;
  },
  knowledge(connection: RuntimeConnection) {
    return [...this.scope(connection), "knowledge"] as const;
  },
  workspaceKnowledge(connection: RuntimeConnection, workspacePath: string) {
    return [...this.knowledge(connection), workspacePath] as const;
  },
  memory(connection: RuntimeConnection) {
    return [...this.scope(connection), "agent-memory"] as const;
  },
  tools(connection: RuntimeConnection) {
    return [...this.scope(connection), "tools", "direct"] as const;
  },
  memoryTarget(
    connection: RuntimeConnection,
    scope: AgentMemoryScope,
    workspacePath?: string,
  ) {
    return [
      ...this.memory(connection),
      scope,
      scope === "project" ? workspacePath : "",
    ] as const;
  },
};

function client(connection: RuntimeConnection): LyraClient {
  return new LyraClient(connection);
}

export async function discoverRuntime(
  connection: RuntimeConnection,
  signal?: AbortSignal,
): Promise<DiscoverResponse> {
  const response = await client(connection).discover(clientMeta, signal);
  if (
    response.protocolVersion !== connection.protocolVersion ||
    response.serverInfo.instanceId !== connection.instanceId ||
    response.capabilities.limits.idempotency.namespace !==
      connection.idempotencyNamespace
  ) {
    throw new Error("Runtime identity changed during discovery");
  }
  return response;
}

export async function listSessions(
  connection: RuntimeConnection,
  cursor?: string,
  signal?: AbortSignal,
): Promise<Page<Session>> {
  return client(connection).call(
    "sessions.list",
    { limit: 100, ...(cursor === undefined ? {} : { cursor }) },
    { meta: clientMeta, signal },
  );
}

export function forkSession(
	connection: RuntimeConnection,
	request: ForkSessionRequest,
): Promise<Session> {
	return client(connection).call("sessions.fork", request, { meta: clientMeta });
}

export function rollbackSession(
	connection: RuntimeConnection,
	request: RollbackSessionRequest,
): Promise<RollbackSessionResponse> {
	return client(connection).call("sessions.rollback", request, { meta: clientMeta });
}

export function exportSession(
	connection: RuntimeConnection,
	sessionId: string,
	format: "json" | "md",
): Promise<ExportSessionResponse> {
	return client(connection).call(
		"sessions.export",
		{ sessionId, format },
		{ meta: clientMeta },
	);
}

export function importSession(
	connection: RuntimeConnection,
	artifact: SessionArtifact,
): Promise<{ session: Session }> {
	return client(connection).call(
		"sessions.import",
		{ artifact },
		{ meta: clientMeta },
	);
}

export function loadSessionUsage(
	connection: RuntimeConnection,
	sessionId: string,
	signal?: AbortSignal,
): Promise<Usage> {
	return client(connection).call(
		"usage.session",
		{ sessionId },
		{ meta: clientMeta, signal },
	);
}

export function listItems(
	connection: RuntimeConnection,
	request: ListItemsRequest,
	signal?: AbortSignal,
): Promise<ListItemsResponse> {
	return client(connection).call("items.list", request, { meta: clientMeta, signal });
}

export function loadUsageSummary(
	connection: RuntimeConnection,
	request: UsageSummaryRequest,
	signal?: AbortSignal,
): Promise<UsageSummary> {
	return client(connection).call(
		"usage.summary",
		request,
		{ meta: clientMeta, signal },
	);
}

export async function createFeedback(
	connection: RuntimeConnection,
	request: FeedbackRequest,
): Promise<void> {
	await client(connection).call("feedback.create", request, { meta: clientMeta });
}

export function listSchedules(
	connection: RuntimeConnection,
	cursor?: string,
	signal?: AbortSignal,
): Promise<Page<Schedule>> {
	return client(connection).call(
		"schedules.list",
		{ limit: 100, ...(cursor === undefined ? {} : { cursor }) },
		{ meta: clientMeta, signal },
	);
}

export function createSchedule(
	connection: RuntimeConnection,
	request: CreateScheduleRequest,
): Promise<Schedule> {
	return client(connection).call("schedules.create", request, { meta: clientMeta });
}

export function updateSchedule(
	connection: RuntimeConnection,
	request: UpdateScheduleRequest,
): Promise<Schedule> {
	return client(connection).call("schedules.update", request, { meta: clientMeta });
}

export function deleteSchedule(
	connection: RuntimeConnection,
	id: string,
): Promise<EmptyObject> {
	return client(connection).call("schedules.delete", { id }, { meta: clientMeta });
}

export function runScheduleNow(
	connection: RuntimeConnection,
	id: string,
): Promise<RunScheduleNowResponse> {
	return client(connection).call("schedules.runNow", { id }, { meta: clientMeta });
}

export function listHooks(
	connection: RuntimeConnection,
	workspace: WorkspaceRef,
	signal?: AbortSignal,
): Promise<HooksListResult> {
	return client(connection).call("hooks.list", { workspace }, { meta: clientMeta, signal });
}

export function setHookTrust(
	connection: RuntimeConnection,
	projectRoot: string,
	trusted: boolean,
): Promise<EmptyObject> {
	return client(connection).call(
		"hooks.setTrust",
		{ projectRoot, trusted },
		{ meta: clientMeta },
	);
}

export function loadSessionSnapshot(
  connection: RuntimeConnection,
  sessionId: string,
  signal?: AbortSignal,
): Promise<SessionSnapshot> {
  return client(connection).call(
    "sessions.snapshot",
    { sessionId, includeDescendants: true },
    { meta: clientMeta, signal },
  );
}

export function listModels(
  connection: RuntimeConnection,
  provider: string,
  signal?: AbortSignal,
): Promise<Page<Model>> {
  return client(connection).call(
    "models.list",
    { provider },
    { meta: clientMeta, signal },
  );
}

export function listProviders(
	connection: RuntimeConnection,
	signal?: AbortSignal,
): Promise<Page<Provider>> {
	return client(connection).call("providers.list", {}, { meta: clientMeta, signal });
}

export function updateProvider(
	connection: RuntimeConnection,
	request: UpdateProviderRequest,
): Promise<Provider> {
	return client(connection).call("providers.update", request, { meta: clientMeta });
}

export function testProvider(
	connection: RuntimeConnection,
	provider: string,
	signal?: AbortSignal,
): Promise<ProviderTestResult> {
	return client(connection).call(
		"providers.test",
		{ provider },
		{ meta: clientMeta, signal },
	);
}

export function getUtilityRole(
	connection: RuntimeConnection,
	signal?: AbortSignal,
): Promise<UtilityRole> {
	return client(connection).call("models.getUtilityRole", {}, { meta: clientMeta, signal });
}

export function setUtilityRole(
	connection: RuntimeConnection,
	role: UtilityRole,
): Promise<UtilityRole> {
	return client(connection).call("models.setUtilityRole", role, { meta: clientMeta });
}

export function getEmbeddingRole(
	connection: RuntimeConnection,
	signal?: AbortSignal,
): Promise<EmbeddingRole> {
	return client(connection).call("models.getEmbeddingRole", {}, { meta: clientMeta, signal });
}

export function setEmbeddingRole(
	connection: RuntimeConnection,
	role: EmbeddingRole,
): Promise<EmbeddingRole> {
	return client(connection).call("models.setEmbeddingRole", role, { meta: clientMeta });
}

export function listMCPServers(
  connection: RuntimeConnection,
  signal?: AbortSignal,
): Promise<Page<MCPServer>> {
  return client(connection).call("mcp.servers.list", {}, { meta: clientMeta, signal });
}

export function createMCPServer(
  connection: RuntimeConnection,
  candidate: MCPServerCandidate,
): Promise<MCPServer> {
  return client(connection).call("mcp.servers.create", candidate, { meta: clientMeta });
}

export function updateMCPServer(
  connection: RuntimeConnection,
  request: UpdateMCPServerRequest,
): Promise<MCPServer> {
  return client(connection).call("mcp.servers.update", request, { meta: clientMeta });
}

export function deleteMCPServer(
  connection: RuntimeConnection,
  server: string,
): Promise<EmptyObject> {
  return client(connection).call("mcp.servers.delete", { server }, { meta: clientMeta });
}

export function testMCPServer(
  connection: RuntimeConnection,
  candidate: MCPServerCandidate,
  signal?: AbortSignal,
): Promise<MCPTestResult> {
  return client(connection).call("mcp.servers.test", candidate, { meta: clientMeta, signal });
}

export function reconnectMCPServer(
  connection: RuntimeConnection,
  server: string,
): Promise<EmptyObject> {
  return client(connection).call("mcp.servers.reconnect", { server }, { meta: clientMeta });
}

export function listMCPTools(
  connection: RuntimeConnection,
  server: string,
  signal?: AbortSignal,
): Promise<Page<MCPTool>> {
  return client(connection).call("mcp.tools.list", { server }, { meta: clientMeta, signal });
}

export function createMCPAuthorizationAttempt(
  connection: RuntimeConnection,
  server: string,
  signal?: AbortSignal,
): Promise<MCPAuthorizationAttempt> {
  return client(connection).call(
    "mcp.authorizationAttempts.create",
    { server },
    { meta: clientMeta, signal },
  );
}

export function getMCPAuthorizationAttempt(
  connection: RuntimeConnection,
  attemptId: string,
  signal?: AbortSignal,
): Promise<MCPAuthorizationAttempt> {
  return client(connection).call(
    "mcp.authorizationAttempts.get",
    { attemptId },
    { meta: clientMeta, signal },
  );
}

export async function authorizeMCPServer(
  connection: RuntimeConnection,
  server: string,
  signal: AbortSignal,
): Promise<MCPAuthorizationAttempt> {
  let attempt = await createMCPAuthorizationAttempt(connection, server, signal);
  while (attempt.status.type === "pending") {
    await abortableDelay(750, signal);
    attempt = await getMCPAuthorizationAttempt(connection, attempt.id, signal);
  }
  return attempt;
}

export function getApprovalMode(
	connection: RuntimeConnection,
	signal?: AbortSignal,
): Promise<ApprovalModeResult> {
	return client(connection).call("approval.getMode", {}, { meta: clientMeta, signal });
}

export function setApprovalMode(
	connection: RuntimeConnection,
	mode: "safe" | "balanced" | "yolo",
): Promise<ApprovalModeResult> {
	return client(connection).call("approval.setMode", { mode }, { meta: clientMeta });
}

export function listApprovalRules(
	connection: RuntimeConnection,
	sessionId: string,
	signal?: AbortSignal,
): Promise<{ rules: ApprovalRule[] }> {
	return client(connection).call(
		"approval.listRules",
		{ sessionId },
		{ meta: clientMeta, signal },
	);
}

export async function forgetApprovalRule(
	connection: RuntimeConnection,
	id: string,
): Promise<void> {
	await client(connection).call("approval.forgetRule", { id }, { meta: clientMeta });
}

export function listWorkspaceFiles(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  path: string,
  cursor?: string,
  signal?: AbortSignal,
): Promise<Page<FileEntry>> {
  return client(connection).call(
    "workspace.files.list",
    {
      workspace,
      path,
      limit: 200,
      ...(cursor === undefined ? {} : { cursor }),
    },
    { meta: clientMeta, signal },
  );
}

export function readWorkspaceFile(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  path: string,
  startLine: number,
  endLine: number,
  signal?: AbortSignal,
): Promise<FileContent> {
  return client(connection).call(
    "workspace.files.read",
    { workspace, path, startLine, endLine, maxBytes: 2 * 1024 * 1024 },
    { meta: clientMeta, signal },
  );
}

export function searchWorkspaceFiles(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  query: string,
  signal?: AbortSignal,
): Promise<GrepResult> {
  return client(connection).call(
    "workspace.files.search",
    { workspace, query, limit: 200 },
    { meta: clientMeta, signal },
  );
}

export function listDiscoveredSkills(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<Skill>> {
  return client(connection).call(
    "skills.discovered.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function listRecipes(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<Recipe>> {
  return client(connection).call(
    "recipes.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function listAgentDocs(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<AgentDoc>> {
  return client(connection).call(
    "agentDocs.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function listDiagnosticTools(
  connection: RuntimeConnection,
  signal?: AbortSignal,
): Promise<Page<ToolSpec>> {
  return client(connection).call(
    "tools.list",
    {},
    { meta: clientMeta, signal },
  );
}

export function invokeDiagnosticTool(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  name: string,
  args: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<unknown> {
  return client(connection).call(
    "tools.invoke",
    { name, arguments: args, workspace },
    { meta: clientMeta, signal },
  );
}

export function listKnowledge(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<KnowledgeEntry>> {
  return client(connection).call(
    "knowledge.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function updateKnowledge(
  connection: RuntimeConnection,
  request: {
    scope: KnowledgeScope;
    workspace?: WorkspaceRef;
    expectedRevision: string;
    content: string;
  },
  signal?: AbortSignal,
): Promise<KnowledgeEntry> {
  return client(connection).call(
    "knowledge.update",
    request,
    { meta: clientMeta, signal },
  );
}

export function listAgentMemory(
  connection: RuntimeConnection,
  scope: AgentMemoryScope,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<AgentMemoryList> {
  return client(connection).call(
    "agentMemory.list",
    scope === "project" ? { scope, workspace } : { scope },
    { meta: clientMeta, signal },
  );
}

export function addAgentMemory(
  connection: RuntimeConnection,
  scope: AgentMemoryScope,
  workspace: WorkspaceRef,
  content: string,
  signal?: AbortSignal,
): Promise<AgentMemoryItem> {
  return client(connection).call(
    "agentMemory.add",
    scope === "project"
      ? { scope, workspace, content }
      : { scope, content },
    { meta: clientMeta, signal },
  );
}

export async function reviewAgentMemory(
  connection: RuntimeConnection,
  id: string,
  decision: AgentMemoryReviewDecision,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "agentMemory.review",
    { id, decision },
    { meta: clientMeta, signal },
  );
}

export function updateAgentMemory(
  connection: RuntimeConnection,
  request: { id: string; content?: string; pinned?: boolean },
  signal?: AbortSignal,
): Promise<AgentMemoryItem> {
  return client(connection).call(
    "agentMemory.update",
    request,
    { meta: clientMeta, signal },
  );
}

export async function deleteAgentMemory(
  connection: RuntimeConnection,
  id: string,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "agentMemory.delete",
    { id },
    { meta: clientMeta, signal },
  );
}

export function listSkillProposals(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<SkillProposal>> {
  return client(connection).call(
    "skills.proposals.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function listManagedSkills(
  connection: RuntimeConnection,
  signal?: AbortSignal,
): Promise<Page<ManagedSkill>> {
  return client(connection).call(
    "skills.library.list",
    {},
    { meta: clientMeta, signal },
  );
}

export async function archiveSkill(
  connection: RuntimeConnection,
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "skills.library.archive",
    { name },
    { meta: clientMeta, signal },
  );
}

export async function restoreSkill(
  connection: RuntimeConnection,
  name: string,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "skills.library.restore",
    { name },
    { meta: clientMeta, signal },
  );
}

export async function approveSkillProposal(
  connection: RuntimeConnection,
  proposal: SkillProposalRef,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "skills.proposals.approve",
    proposal,
    { meta: clientMeta, signal },
  );
}

export async function rejectSkillProposal(
  connection: RuntimeConnection,
  proposal: SkillProposalRef,
  signal?: AbortSignal,
): Promise<void> {
  await client(connection).call(
    "skills.proposals.reject",
    proposal,
    { meta: clientMeta, signal },
  );
}

export function listWorkspaceChanges(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<Page<WorkspaceFileChange>> {
  return client(connection).call(
    "workspace.changes.list",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function getWorkspaceDiff(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  path: string,
  mode: "worktree" | "base",
  signal?: AbortSignal,
): Promise<Diff> {
  return client(connection).call(
    "workspace.diff.get",
    { workspace, path, mode, format: "rows", limit: 100_000 },
    { meta: clientMeta, signal },
  );
}

export function getCodebaseStatus(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  signal?: AbortSignal,
): Promise<CodebaseStatus> {
  return client(connection).call(
    "codebase.status",
    { workspace },
    { meta: clientMeta, signal },
  );
}

export function searchCodebase(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
  query: string,
  signal?: AbortSignal,
): Promise<CodebaseSearchResult> {
  return client(connection).call(
    "codebase.search",
    { workspace, query, limit: 12 },
    { meta: clientMeta, signal },
  );
}

export function reindexCodebase(
  connection: RuntimeConnection,
  workspace: WorkspaceRef,
): Promise<CodebaseReindexResponse> {
  return client(connection).call(
    "codebase.reindex",
    { workspace },
    { meta: clientMeta },
  );
}

export function startRun(
  connection: RuntimeConnection,
  sessionId: string,
  input: ContentBlock[],
  idempotencyKey: string,
  selection?: { provider: string; model: string },
  signal?: AbortSignal,
): Promise<OpenRuntimeStream<StartRunResponse, RunEvent>> {
  return client(connection).stream(
    "runs.start",
		{ sessionId, input, ...(selection === undefined ? {} : selection) },
    { meta: clientMeta, idempotencyKey, signal },
  );
}

export function resumeRun(
  connection: RuntimeConnection,
  runId: string,
  responses: InterruptResponse[],
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<OpenRuntimeStream<ResumeRunResponse, RunEvent>> {
  const request: ResumeRunRequest = { runId, responses };
  return client(connection).stream(
    "runs.resume",
    request,
    { meta: clientMeta, idempotencyKey, signal },
  );
}

export function subscribeRun(
  connection: RuntimeConnection,
  runId: string,
  segmentId: string,
  signal?: AbortSignal,
  afterEventId?: string,
) {
  return client(connection).stream(
    "runs.subscribe",
    { runId, segmentId },
    { meta: clientMeta, signal, afterEventId },
  );
}

export function steerRun(
  connection: RuntimeConnection,
  runId: string,
  segmentId: string,
  input: ContentBlock[],
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<EmptyObject> {
  return client(connection).call(
    "runs.steer",
    { runId, expectedSegmentId: segmentId, input },
    { meta: clientMeta, idempotencyKey, signal },
  );
}

export function cancelRun(
  connection: RuntimeConnection,
  runId: string,
  signal?: AbortSignal,
): Promise<CancelRunResponse> {
  return client(connection).call(
    "runs.cancel",
    { runId, reason: "Stopped from Lyra Desktop" },
    { meta: clientMeta, signal },
  );
}

export function createSession(
  connection: RuntimeConnection,
  request: CreateSessionRequest = {},
): Promise<Session> {
  return client(connection).call(
    "sessions.create",
    request,
    { meta: clientMeta },
  );
}

export function updateSession(
  connection: RuntimeConnection,
  request: UpdateSessionRequest,
): Promise<Session> {
  return client(connection).call("sessions.update", request, {
    meta: clientMeta,
  });
}

export function deleteSession(
  connection: RuntimeConnection,
  sessionId: string,
): Promise<EmptyObject> {
  return client(connection).call(
    "sessions.delete",
    { sessionId },
    { meta: clientMeta },
  );
}

export function startGoal(
  connection: RuntimeConnection,
  sessionId: string,
  objective: string,
  budget?: GoalBudget,
): Promise<Goal> {
  return client(connection).call(
    "goals.start",
    { sessionId, objective, ...(budget === undefined ? {} : { budget }) },
    { meta: clientMeta },
  );
}

export function updateGoal(
  connection: RuntimeConnection,
  sessionId: string,
  objective: string,
): Promise<Goal> {
  return client(connection).call(
    "goals.update",
    { sessionId, objective },
    { meta: clientMeta },
  );
}

export function pauseGoal(
  connection: RuntimeConnection,
  sessionId: string,
): Promise<Goal> {
  return client(connection).call(
    "goals.stop",
    { sessionId },
    { meta: clientMeta },
  );
}

export function resumeGoal(
  connection: RuntimeConnection,
  sessionId: string,
): Promise<Goal> {
  return client(connection).call(
    "goals.resume",
    { sessionId },
    { meta: clientMeta },
  );
}

export async function clearGoal(
  connection: RuntimeConnection,
  sessionId: string,
): Promise<void> {
  await client(connection).call(
    "goals.clear",
    { sessionId },
    { meta: clientMeta },
  );
}

const runtimeInvalidationTopics = [
	"sessions.changed",
	"runs.changed",
	"plan.changed",
	"goals.changed",
	"interrupts.changed",
	"models.changed",
	"mcp.changed",
	"approvals.changed",
	"schedules.changed",
	"files.changed",
	"skills.changed",
	"knowledge.changed",
	"hooks.changed",
	"agentMemory.changed",
	"codebase.changed",
] satisfies RuntimeTopic[];

export async function consumeRuntimeInvalidations(
  connection: RuntimeConnection,
  signal: AbortSignal,
  onOpen: () => void,
  onEvent: (event: RuntimeEvent) => void,
  watch?: { id: string; workspace: WorkspaceRef },
): Promise<void> {
  const stream = await client(connection).stream(
    "runtime.subscribe",
    {
		topics: runtimeInvalidationTopics,
      ...(watch === undefined
        ? {}
        : { watches: [{ watchId: watch.id, workspace: watch.workspace }] }),
    },
    { meta: clientMeta, signal },
  );
  onOpen();
	let sequence = 0;
	for await (const frame of stream) {
		if (frame.event.sequence !== sequence + 1) {
			onEvent({
				type: "resync",
				sequence: frame.event.sequence,
				topics: [...runtimeInvalidationTopics],
				...(watch === undefined ? {} : { watchIds: [watch.id] }),
			});
		}
		sequence = frame.event.sequence;
		onEvent(frame.event);
	}
}

function abortableDelay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason);
      return;
    }
    const timer = window.setTimeout(finish, milliseconds);
    function finish() {
      signal.removeEventListener("abort", abort);
      resolve();
    }
    function abort() {
      window.clearTimeout(timer);
      reject(signal.reason);
    }
    signal.addEventListener("abort", abort, { once: true });
  });
}
