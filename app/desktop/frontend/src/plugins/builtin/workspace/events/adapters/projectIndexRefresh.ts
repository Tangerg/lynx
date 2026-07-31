import { queryClient } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { WORKSPACE_PROJECTS_KEY } from "@/plugins/builtin/workspace/public/queries";

/**
 * Keep the project index fresh as the session collection moves.
 *
 * A project row IS a view over the agent's sessions — their cwds, and how many
 * sessions each one holds — so it goes stale exactly when that collection does.
 * The agent used to announce this itself, by invalidating the workspace's query
 * key spelled as the literal `"projects"`: importing the owner's constant would
 * have made the two contexts circular, so the string hid the edge that the cycle
 * check would otherwise have refused.
 *
 * The dependency belongs this way round. The workspace already watches this same
 * cache entry to resolve the active cwd (see `sessionWorkspaceCwd`), so watching
 * it for the index it derives adds no new coupling — and the agent goes back to
 * knowing only about its own sessions.
 */
export function installProjectIndexRefresh(): () => void {
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] !== AGENT_SESSIONS_KEY) return;
    void queryClient.invalidateQueries({ queryKey: [WORKSPACE_PROJECTS_KEY] });
  });
}
