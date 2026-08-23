import {
  LyraClient,
  protocolVersion,
  type DiscoverResponse,
  type CancelRunResponse,
  type ContentBlock,
  type CreateSessionRequest,
  type EmptyObject,
  type Goal,
  type GoalBudget,
  type InterruptResponse,
  type Model,
  type Page,
  type RequestMeta,
  type ResumeRunRequest,
  type ResumeRunResponse,
  type RuntimeEvent,
  type RuntimeConnection,
  type OpenRuntimeStream,
  type RunEvent,
  type Session,
  type SessionSnapshot,
  type StartRunResponse,
  type UpdateSessionRequest,
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
      ],
    },
    { meta: clientMeta, signal },
  );
  onOpen();
  for await (const frame of stream) onEvent(frame.event);
}
