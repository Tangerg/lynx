import {
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { WORKSPACE_PROJECTS_KEY } from "@/plugins/builtin/workspace/public/queries";
import { replaceCachedRead } from "./queryInvalidation";

/**
 * Keep the project index fresh when the Session facts it is derived from move.
 *
 * A project row IS a view over the agent's sessions — their cwds, and how many
 * sessions each one holds — so it goes stale exactly when that collection does.
 * The agent used to announce this itself, by invalidating the workspace's query
 * key spelled as the literal `"projects"`: importing the owner's constant would
 * have made the two contexts circular, so the string hid the edge that the cycle
 * check would otherwise have refused.
 *
 * Query-cache lifecycle events are deliberately not the signal. The project
 * read depends only on Session identity, cwd, and updated time; status changes,
 * observer churn, and fetch-state changes cannot alter it and must not refetch
 * workspaces.list.
 */
export function installProjectIndexRefresh(): () => void {
  return subscribeAgentSessionProjection(workspaceProjectRevision, () => {
    replaceCachedRead({ queryKey: [WORKSPACE_PROJECTS_KEY] });
  });
}

export function workspaceProjectRevision(
  sessions: readonly AgentSessionSummary[] | undefined,
): string {
  return JSON.stringify(sessions?.map(({ id, cwd, time }) => [id, cwd ?? "", time]) ?? null);
}
