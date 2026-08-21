import type { AgentProjectionMaterial } from "./sessionView";
import { agentSessionView } from "./sessionView";

export type AgentSessionSharedMaterialContributor<T> = (sessionId: string, material: T) => unknown;

interface RegisteredContributor {
  active: boolean;
  key: string;
  project(sessionId: string, material: unknown): unknown;
}

const contributors = new Map<string, RegisteredContributor>();

/**
 * Extend the mounted Agent Session's generic shared material without teaching
 * the Agent inner ring another bounded context's vocabulary. Each key has one
 * owner and is projected before the same view-token commit as Run/HITL/Plan/Tool.
 */
export function registerAgentSessionSharedMaterial<T>(
  key: string,
  project: AgentSessionSharedMaterialContributor<T>,
): () => void {
  if (contributors.has(key)) {
    throw new Error(`Agent Session shared material "${key}" already has an owner`);
  }
  const registered: RegisteredContributor = {
    active: true,
    key,
    project: (sessionId, material) => project(sessionId, material as T),
  };
  contributors.set(key, registered);
  return () => {
    registered.active = false;
    if (contributors.get(key) === registered) contributors.delete(key);
  };
}

/** Stage pure companion values beside one Runtime snapshot. A plugin disposed
 * before the snapshot wins contributes nothing; no staged function writes a
 * store of its own. */
export function stageAgentSessionSharedMaterial<T>(
  sessionId: string,
  material: T,
): (shared: Record<string, unknown>) => Record<string, unknown> {
  const staged = [...contributors.values()].map((registered) => ({
    registered,
    value: registered.project(sessionId, material),
  }));
  return (shared) => {
    let projected = shared;
    for (const { registered, value } of staged) {
      if (!registered.active) continue;
      projected = { ...projected, [registered.key]: value };
    }
    return projected;
  };
}

/** Read one companion value together with the exact active Session generation
 * that admitted it. */
export function useAgentSessionSharedMaterial<T>(path: string): AgentProjectionMaterial<T> {
  return agentSessionView().useSharedMaterial<T>(path);
}

/** Read one already-mounted companion value at an action boundary. This is the
 * imperative sibling of `useAgentSessionSharedMaterial`; it never starts a
 * query or writes a second projection. */
export function getAgentSessionSharedMaterial<T>(sessionId: string, path: string): T | undefined {
  return agentSessionView().getSession(sessionId)?.view.shared[path] as T | undefined;
}
