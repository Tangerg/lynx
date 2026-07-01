// @codebase actions (codebase.search / reindex). Both refresh the codebase
// status query so the view's status header reflects the new index state.

import type { CodebaseHit } from "@/rpc";
import { CODEBASE_STATUS_KEY, useCodebaseStatus, useEmbeddingRole } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";

export type CodebaseSearchHit = CodebaseHit;

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
): Promise<CodebaseHit[]> {
  const res = await getContainer().client().codebase.search({ cwd, query, limit });
  await queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] });
  return res.hits;
}

export async function reindexCodebase(cwd: string | undefined): Promise<void> {
  await getContainer().client().codebase.reindex(cwd);
  await queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] });
}
