import type { RuntimeEvent, RuntimeTopic } from "@/rpc";
import { getContainer } from "@/main/container";
import { RUNTIME_SUBSCRIBE_METHOD } from "@/rpc/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
} from "@/plugins/builtin/runtime/public/capabilities";

// Every topic, because this client now has a read for every one of them — the run
// stream reaches only the window driving that run, so a session moved by the
// autonomous loop, another window, or the scheduler arrives here or not at all.
// There is no wildcard by design (§7.2): a subscription says what it can fold, and
// this list is checked against the reducer's closed set at the call below.
const SUBSCRIBED_TOPICS: readonly RuntimeTopic[] = [
  "files.changed",
  "skills.changed",
  "mcp.changed",
  "schedules.changed",
  "sessions.changed",
  "runs.changed",
  "interrupts.changed",
  "goals.changed",
  "state.changed",
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
