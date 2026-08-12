import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  getActiveSessionId,
  subscribeAgentSessionProjection,
  subscribeActiveSessionId,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { asSessionId, isErrorType } from "@/rpc";
import type {
  WorkspaceCwdInputChange,
  WorkspaceCwdResolution,
} from "../application/workspaceEventSubscription";

export async function resolveActiveSessionWorkspaceCwd(
  signal: AbortSignal,
): Promise<WorkspaceCwdResolution> {
  const id = getActiveSessionId();
  if (!id) return { status: "resolved" };
  const list = queryClient.getQueryData<{ id: string; cwd?: string }[]>([AGENT_SESSIONS_KEY]);
  const cached = list?.find((session) => session.id === id);
  if (cached) return { status: "resolved", ...(cached.cwd ? { cwd: cached.cwd } : {}) };
  return getContainer()
    .client()
    .sessions.get(asSessionId(id), signal)
    .then((session) => ({ status: "resolved", cwd: session.workspace.ref.path }) as const)
    .catch((error: unknown) => {
      if (isErrorType(error, "session_not_found")) return { status: "unavailable" } as const;
      throw error;
    });
}

export function subscribeWorkspaceCwdInputs(
  onChange: (change: WorkspaceCwdInputChange) => void,
): () => void {
  const unsubSession = subscribeActiveSessionId(() => onChange("identity"));
  const unsubCache = subscribeAgentSessionProjection(sessionWorkspaceRevision, () =>
    onChange("projection"),
  );
  return () => {
    unsubSession();
    unsubCache();
  };
}

function sessionWorkspaceRevision(sessions: readonly AgentSessionSummary[] | undefined): string {
  const active = getActiveSessionId();
  const session = sessions?.find(({ id }) => id === active);
  return JSON.stringify([active, session ? session.cwd : null]);
}
