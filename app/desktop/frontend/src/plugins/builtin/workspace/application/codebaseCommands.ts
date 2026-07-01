// @codebase actions (codebase.search / reindex). Both refresh the codebase
// status query so the view's status header reflects the new index state.

import { CODEBASE_STATUS_KEY, useCodebaseStatus, useEmbeddingRole } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { codebaseGateway, type CodebaseSearchHit } from "./ports/codebaseGateway";

export type { CodebaseSearchHit } from "./ports/codebaseGateway";

export function useCodebaseSearchConfig() {
  const cwd = useActiveSessionCwd();
  const { data: role } = useEmbeddingRole();
  const { data: status } = useCodebaseStatus({ cwd });
  return {
    cwd,
    status,
    enabled: Boolean(role?.model),
  };
}

export async function searchCodebase(
  cwd: string | undefined,
  query: string,
  limit = 12,
): Promise<CodebaseSearchHit[]> {
  const hits = await codebaseGateway().search({ cwd, query, limit });
  await queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] });
  return hits;
}

export async function reindexCodebase(cwd: string | undefined): Promise<void> {
  await codebaseGateway().reindex(cwd);
  await queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] });
}
