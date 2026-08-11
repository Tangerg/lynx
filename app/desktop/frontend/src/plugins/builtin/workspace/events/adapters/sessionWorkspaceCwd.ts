import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  getActiveSessionId,
  subscribeAgentSessionProjection,
  subscribeActiveSessionId,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { asSessionId } from "@/rpc";
import type { WorkspaceCwdResolution } from "../application/workspaceEventSubscription";

export async function resolveActiveSessionWorkspaceCwd(): Promise<WorkspaceCwdResolution> {
  const id = getActiveSessionId();
  if (!id) return { status: "resolved" };
  const list = queryClient.getQueryData<{ id: string; cwd?: string }[]>([AGENT_SESSIONS_KEY]);
  const cached = list?.find((session) => session.id === id);
  if (cached) return { status: "resolved", ...(cached.cwd ? { cwd: cached.cwd } : {}) };
  return getContainer()
    .client()
    .sessions.get(asSessionId(id))
    .then((session) => ({ status: "resolved", cwd: session.workspace.ref.path }) as const)
    .catch(() => ({ status: "unavailable" }) as const);
}

export function subscribeWorkspaceCwdInputs(onChange: () => void): () => void {
  const unsubSession = subscribeActiveSessionId(onChange);
  const unsubCache = subscribeAgentSessionProjection(sessionWorkspaceRevision, onChange);
  return () => {
    unsubSession();
    unsubCache();
  };
}

function sessionWorkspaceRevision(sessions: readonly AgentSessionSummary[] | undefined): string {
  return JSON.stringify(sessions?.map(({ id, cwd }) => [id, cwd ?? ""]) ?? null);
}
