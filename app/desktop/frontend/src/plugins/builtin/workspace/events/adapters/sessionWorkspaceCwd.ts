import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/data/queryClient";
import {
  AGENT_SESSIONS_KEY,
  getActiveSessionId,
  subscribeActiveSessionId,
} from "@/plugins/builtin/agent/public/session";
import { asSessionId } from "@/rpc";

export async function resolveActiveSessionWorkspaceCwd(): Promise<string | undefined> {
  const id = getActiveSessionId();
  if (!id) return undefined;
  const list = queryClient.getQueryData<{ id: string; cwd?: string }[]>([AGENT_SESSIONS_KEY]);
  const cached = list?.find((session) => session.id === id)?.cwd;
  if (cached !== undefined || list !== undefined) return cached;
  return getContainer()
    .client()
    .sessions.get(asSessionId(id))
    .then((session) => session.cwd)
    .catch(() => undefined);
}

export function subscribeWorkspaceCwdInputs(onChange: () => void): () => void {
  const unsubSession = subscribeActiveSessionId(onChange);
  const unsubCache = queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] === AGENT_SESSIONS_KEY) onChange();
  });
  return () => {
    unsubSession();
    unsubCache();
  };
}
