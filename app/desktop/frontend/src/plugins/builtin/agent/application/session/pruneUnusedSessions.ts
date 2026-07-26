import { t } from "@/lib/i18n";
import { notifyInfo } from "@/plugins/sdk";
import type { AgentSessionSummary } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { invalidateAgentSessions } from "./sessionQueries";

/**
 * Sweep the sessions that hold nothing, once per launch.
 *
 * The leaks that produced them are fixed, but the pile they left is on disk, and
 * every one of those sessions is a row the user has to look at and delete by hand.
 * A session the app abandoned while it was still a local "draft" is worse than
 * ordinary clutter: the draft mark is ephemeral, so after a restart it stops being
 * hidden and simply appears as an untitled, empty session.
 *
 * Two stages, because deleting a session must never rest on a guess:
 *
 *  - narrow on a fact from the list: the runtime titles a session when a run
 *    finishes (RenameIfUntitled), so a still-empty title means no run of this
 *    session has ever completed. Favourites are excluded — pinning is the user
 *    saying they want it — and so is anything the app currently holds open,
 *    which includes the session being restored into view right now.
 *  - then ASK, per candidate: one transcript row is enough to know the session
 *    holds something. Only a session the runtime confirms is empty is deleted.
 *
 * The count is reported rather than swept under the rug: deletions the user did
 * not ask for should never be silent, even when what went was nothing.
 */
export async function pruneUnusedSessions(
  sessions: readonly AgentSessionSummary[],
  openSessionIds: readonly string[],
): Promise<number> {
  const open = new Set(openSessionIds);
  const candidates = sessions.filter(
    (session) => session.title === "" && !session.favorite && !open.has(session.id),
  );
  if (candidates.length === 0) return 0;

  const runtime = agentRuntime();
  let removed = 0;
  for (const candidate of candidates) {
    try {
      if (!(await runtime.sessionHoldsNothing(candidate.id))) continue;
      await runtime.deleteSession(candidate.id);
      removed += 1;
    } catch (err) {
      // Housekeeping: a session that refuses to go stays, and the next launch can
      // try again. Nothing here is worth interrupting the user for.
      console.warn("[session] pruning an unused session failed:", candidate.id, err);
    }
  }
  if (removed > 0) {
    await invalidateAgentSessions();
    notifyInfo(t("session.pruned", { count: removed }), { source: "session" });
  }
  return removed;
}
