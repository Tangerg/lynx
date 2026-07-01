import type { WorkspaceEvent } from "@/rpc";
import { getContainer } from "@/main/container";
import { WORKSPACE_SUBSCRIBE_METHOD } from "@/rpc/transport";
import { serverFeature, useRuntimeStore } from "@/state/runtimeStore";

export function canSubscribeWorkspaceEvents(): boolean {
  return (
    useRuntimeStore
      .getState()
      .capabilities?.streamingMethods?.includes(WORKSPACE_SUBSCRIBE_METHOD) ?? false
  );
}

export function subscribeRuntimeCapabilities(onChange: () => void): () => void {
  return useRuntimeStore.subscribe(onChange);
}

export async function subscribeRuntimeWorkspaceEvents(
  cwd: string | undefined,
  signal: AbortSignal,
): Promise<AsyncIterable<WorkspaceEvent>> {
  const fileWatch = serverFeature("fileWatch");
  const { events } = await getContainer()
    .client()
    .workspace.subscribe(
      fileWatch ? { watches: [{ watchId: "active-session", ...(cwd ? { cwd } : {}) }] } : undefined,
      signal,
    );
  return events;
}
