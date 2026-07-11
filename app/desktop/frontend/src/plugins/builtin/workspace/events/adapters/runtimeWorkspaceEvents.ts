import type { WorkspaceEvent } from "@/rpc";
import { getContainer } from "@/main/container";
import { WORKSPACE_SUBSCRIBE_METHOD } from "@/rpc/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
} from "@/plugins/builtin/runtime/public/capabilities";

export function canSubscribeWorkspaceEvents(): boolean {
  return runtimeSupportsStreamingMethod(WORKSPACE_SUBSCRIBE_METHOD);
}

export async function subscribeRuntimeWorkspaceEvents(
  cwd: string | undefined,
  signal: AbortSignal,
): Promise<AsyncIterable<WorkspaceEvent>> {
  const fileWatch = runtimeCapability("fileWatch");
  const { events } = await getContainer()
    .client()
    .workspace.subscribe(
      fileWatch ? { watches: [{ watchId: "active-session", ...(cwd ? { cwd } : {}) }] } : undefined,
      signal,
    );
  return events;
}
