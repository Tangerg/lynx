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
  type Diff,
  type EmptyObject,
  type FileContent,
  type FileEntry,
  type AgentDoc,
  type Goal,
  type GoalBudget,
  type GrepResult,
  type InterruptResponse,
  type ManagedSkill,
  type Model,
  type Page,
  type RequestMeta,
  type Recipe,
  type ResumeRunRequest,
  type ResumeRunResponse,
  type RuntimeEvent,
  type RuntimeConnection,
  type OpenRuntimeStream,
  type RunEvent,
  type Session,
  type SessionSnapshot,
  type Skill,
  type SkillProposal,
  type SkillProposalRef,
  type StartRunResponse,
  type UpdateSessionRequest,
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
  snapshot(connection: RuntimeConnection, sessionId: string) {
    return [...this.scope(connection), "session", sessionId, "snapshot"] as const;
  },
  models(connection: RuntimeConnection, provider: string) {
    return [...this.scope(connection), "models", provider] as const;
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
  signal?: AbortSignal,
): Promise<OpenRuntimeStream<StartRunResponse, RunEvent>> {
  return client(connection).stream(
    "runs.start",
    { sessionId, input },
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
      topics: [
        "sessions.changed",
        "runs.changed",
        "plan.changed",
        "goals.changed",
        "interrupts.changed",
        "models.changed",
        "files.changed",
        "skills.changed",
        "codebase.changed",
      ],
      ...(watch === undefined
        ? {}
        : { watches: [{ watchId: watch.id, workspace: watch.workspace }] }),
    },
    { meta: clientMeta, signal },
  );
  onOpen();
  for await (const frame of stream) onEvent(frame.event);
}
