// @codebase actions (codebase.search / reindex). Lives in lib/ so the Codebase
// workspace view (a component) reaches the runtime through these helpers rather
// than importing @/rpc / @/main directly (layer rule). Both refresh the
// codebase-status query so the view's status header reflects the new index state.

import type { CodebaseHit } from "@/rpc";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";

export async function searchCodebase(
  cwd: string | undefined,
  query: string,
  limit = 12,
): Promise<CodebaseHit[]> {
  const res = await getContainer().client().codebase.search({ cwd, query, limit });
  await queryClient.invalidateQueries({ queryKey: ["codebase-status"] });
  return res.hits;
}

export async function reindexCodebase(cwd: string | undefined): Promise<void> {
  await getContainer().client().codebase.reindex(cwd);
  await queryClient.invalidateQueries({ queryKey: ["codebase-status"] });
}
