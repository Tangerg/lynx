import type { AgentCancelResult } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { settleRunPendingInterrupts } from "./fold";
import { foldRunSnapshot } from "./runSnapshot";

/** Merge only facts the cancel command committed. Descendant lifecycle not
 * present in a root response remains query-owned and arrives through the
 * authoritative refresh; no local terminal state is invented. */
export function foldCancelRunResponse(
  state: AgentSessionView,
  response: AgentCancelResult,
): AgentSessionView {
  if (response.type === "child") {
    const withChild = foldRunSnapshot(state, response.run);
    const withRoot = foldRunSnapshot(withChild, response.rootRun);
    return settleRunPendingInterrupts(withRoot, response.run.id);
  }

  let next = foldRunSnapshot(state, response.run);
  for (const run of Object.values(state.runsById)) {
    if (run.rootRunId === response.run.id) {
      next = settleRunPendingInterrupts(next, run.id);
    }
  }
  return next;
}
