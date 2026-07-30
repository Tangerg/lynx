import type { StateSnapshot } from "@/rpc";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";

// A snapshot is a whole latest value, keyed by its own `type` — not by the run that
// wrote it. The envelope's runId is provenance: a session-scoped list bucketed per
// run would split one checklist into one per turn.
//
// The revision decides which of two snapshots is later. Contents cannot: the list is
// replaced wholesale, so it can shrink, and an out-of-order older snapshot would look
// exactly like progress being undone. Dropping the older one is what makes the fold
// order-insensitive — which it has to be, because a live stream and a cold read can
// both deliver one.
export function onStateSnapshot(
  state: AgentSessionView,
  snapshot: StateSnapshot,
): AgentSessionView {
  if (supersededBy(state.shared[snapshot.type], snapshot)) return state;
  return { ...state, shared: { ...state.shared, [snapshot.type]: snapshot } };
}

function supersededBy(held: unknown, arriving: StateSnapshot): boolean {
  if (held === null || typeof held !== "object" || !("revision" in held)) return false;
  const revision = (held as { revision: unknown }).revision;
  return typeof revision === "number" && revision > arriving.revision;
}
