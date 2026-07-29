import type { RuntimeEvent, RuntimeTopic } from "@/rpc";
import { getContainer } from "@/main/container";
import { RUNTIME_SUBSCRIBE_METHOD } from "@/rpc/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
} from "@/plugins/builtin/runtime/public/capabilities";

// The topics this client can act on: each one maps to a read it invalidates. It does
// NOT ask for runs / interrupts / goals / state — those are folded from the run stream
// today, so a signal about them would ask for a refetch of nothing. There is no
// wildcard by design (§7.2): a subscription says what it can fold.
const SUBSCRIBED_TOPICS: readonly RuntimeTopic[] = [
  "files.changed",
  "skills.changed",
  "mcp.changed",
  "schedules.changed",
  "sessions.changed",
];

export function canSubscribeWorkspaceEvents(): boolean {
  return runtimeSupportsStreamingMethod(RUNTIME_SUBSCRIBE_METHOD);
}

export async function subscribeRuntimeWorkspaceEvents(
  cwd: string | undefined,
  signal: AbortSignal,
): Promise<AsyncIterable<RuntimeEvent>> {
  // Watches are legal only alongside files.changed, and only when the runtime offers
  // the capability that produces them.
  const watches = runtimeCapability("fileWatch")
    ? [{ watchId: "active-session", ...(cwd ? { cwd } : {}) }]
    : undefined;
  const { events } = await getContainer()
    .client()
    .runtimeEvents.subscribe(
      { topics: [...SUBSCRIBED_TOPICS], ...(watches ? { watches } : {}) },
      signal,
    );
  return events;
}
