import type { RuntimeEvent, RuntimeTopic } from "@/rpc";
import { getContainer } from "@/main/container";
import { RUNTIME_SUBSCRIBE_METHOD } from "@/rpc/transport";
import {
  runtimeCapability,
  runtimeSupportsStreamingMethod,
  runtimeSupportsTopic,
} from "@/plugins/builtin/runtime/public/capabilities";
import type { WorkspaceWatchTarget } from "../application/workspaceEventLoop";

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
  "plan.changed",
  "knowledge.changed",
  "hooks.changed",
  "models.changed",
  "approvals.changed",
  "agentMemory.changed",
];

export function canSubscribeWorkspaceEvents(): boolean {
  return runtimeSupportsStreamingMethod(RUNTIME_SUBSCRIBE_METHOD);
}

export async function subscribeRuntimeWorkspaceEvents(
  target: WorkspaceWatchTarget,
  signal: AbortSignal,
): Promise<AsyncIterable<RuntimeEvent>> {
  // A file watch is an optional scope on the app-wide signal stream. Resolve it
  // before subscribing so a workspace which disappeared after its session was
  // opened simply contributes no watch; it must not take the global sessions,
  // runs, HITL, goal, plan, MCP, and schedule invalidations offline with it.
  // Other resolution failures still reject and enter the subscription loop's
  // reconnect path.
  const client = getContainer().client();
  const workspace =
    runtimeCapability("fileWatch") && target.type === "workspace"
      ? await client.workspaces.resolve(target.cwd ? { path: target.cwd } : undefined, signal)
      : undefined;
  const watches =
    workspace?.availability === "available"
      ? [{ watchId: "active-session", workspace: { path: workspace.ref.path } }]
      : undefined;
  // The client-owned list states what this build can fold. Discovery states what
  // this Runtime accepts. Their intersection is the only honest subscription:
  // requesting a newer topic from an older Runtime rejects the whole stream and
  // takes unrelated file/session/run invalidations offline.
  const topics = SUBSCRIBED_TOPICS.filter(runtimeSupportsTopic);
  const { events } = await client.runtimeEvents.subscribe(
    { topics, ...(watches ? { watches } : {}) },
    signal,
  );
  return events;
}
