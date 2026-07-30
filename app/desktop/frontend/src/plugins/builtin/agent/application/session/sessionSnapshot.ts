import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionSnapshot } from "../ports/runtimeGateway";
import { reduceDurableItem } from "../fold/reducer";
import { foldRunSnapshot } from "../fold/runSnapshot";
import { foldPendingInterruptSet } from "../fold/pendingInterruptSnapshot";
import { onStateSnapshot } from "../fold/stateHandlers";

/**
 * Project one complete durable read off-store. Callers either commit the
 * returned value wholesale or discard it; partially fetched state never
 * becomes observable.
 */
export function projectAgentSessionSnapshot(snapshot: AgentSessionSnapshot): AgentSessionView {
  let view = EMPTY_AGENT_SESSION_VIEW;
  for (const run of snapshot.runs) view = foldRunSnapshot(view, run);
  for (const item of snapshot.items) view = reduceDurableItem(view, item);
  for (const pending of snapshot.pendingInterruptSets) {
    view = foldPendingInterruptSet(view, pending);
  }
  if (snapshot.state) view = onStateSnapshot(view, snapshot.state);
  return view;
}
