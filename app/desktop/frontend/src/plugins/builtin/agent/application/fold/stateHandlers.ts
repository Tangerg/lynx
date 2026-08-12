import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentPlanStateSnapshot } from "../../domain/plan";

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
  snapshot: AgentPlanStateSnapshot,
): AgentSessionView {
  if (supersededBy(state.shared[snapshot.type], snapshot)) return state;
  return { ...state, shared: { ...state.shared, [snapshot.type]: snapshot } };
}

function supersededBy(held: unknown, arriving: AgentPlanStateSnapshot): boolean {
  if (held === null || typeof held !== "object" || !("revision" in held)) return false;
  const revision = (held as { revision: unknown }).revision;
  // Equal revision is the same version, not a later write. Treat its replay as
  // a no-op as well; otherwise two same-revision frames with drifted contents
  // make the final projection depend on arrival order.
  return typeof revision === "number" && revision >= arriving.revision;
}
