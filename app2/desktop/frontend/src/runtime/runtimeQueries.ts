import {
  LyraClient,
  protocolVersion,
  type DiscoverResponse,
  type Goal,
  type GoalBudget,
  type RequestMeta,
  type RuntimeEvent,
  type RuntimeConnection,
  type Session,
  type SessionSnapshot,
} from "@lyra/runtime-contract";

const clientMeta: RequestMeta = {
  protocolVersion,
  clientInfo: { name: "lyra-desktop-app2", version: "0.0.0" },
  clientCapabilities: {
    features: { plan: { enabled: true }, goals: { enabled: true } },
    interruptTypes: [],
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
  signal?: AbortSignal,
): Promise<Session[]> {
  const page = await client(connection).call(
    "sessions.list",
    { limit: 100 },
    { meta: clientMeta, signal },
  );
  return page.data;
}

export function loadSessionSnapshot(
  connection: RuntimeConnection,
  sessionId: string,
  signal?: AbortSignal,
): Promise<SessionSnapshot> {
  return client(connection).call(
    "sessions.snapshot",
    { sessionId },
    { meta: clientMeta, signal },
  );
}

export function createSession(
  connection: RuntimeConnection,
  title?: string,
): Promise<Session> {
  return client(connection).call(
    "sessions.create",
    { ...(title === undefined ? {} : { title }) },
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
    { topics: ["sessions.changed", "plan.changed", "goals.changed"] },
    { meta: clientMeta, signal },
  );
  onOpen();
  for await (const frame of stream) onEvent(frame.event);
}
