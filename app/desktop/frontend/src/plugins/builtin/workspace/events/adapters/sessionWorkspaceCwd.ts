import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/data/queryClient";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";

export async function resolveActiveSessionWorkspaceCwd(): Promise<string | undefined> {
  const id = useSessionStore.getState().activeSessionId;
  if (!id) return undefined;
  const list = queryClient.getQueryData<{ id: string; cwd?: string }[]>([SESSIONS_KEY]);
  const cached = list?.find((session) => session.id === id)?.cwd;
  if (cached !== undefined || list !== undefined) return cached;
  return getContainer()
    .client()
    .sessions.get(asSessionId(id))
    .then((session) => session.cwd)
    .catch(() => undefined);
}

export function subscribeWorkspaceCwdInputs(onChange: () => void): () => void {
  const unsubSession = useSessionStore.subscribe(onChange);
  const unsubCache = queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] === SESSIONS_KEY) onChange();
  });
  return () => {
    unsubSession();
    unsubCache();
  };
}
