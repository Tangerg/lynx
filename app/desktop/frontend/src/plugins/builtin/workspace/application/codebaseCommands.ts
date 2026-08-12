// @codebase actions (codebase.search / reindex). Both refresh the codebase
// status query so the view's status header reflects the new index state.

import { queryClient } from "@/lib/queryClient";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  CODEBASE_STATUS_KEY,
  useCodebaseStatus,
  useEmbeddingRole,
} from "@/plugins/builtin/settings/providers/public/queries";
import { codebaseGateway, type CodebaseSearchHit } from "./ports/codebaseGateway";

export type { CodebaseSearchHit } from "./ports/codebaseGateway";

export function useCodebaseSearchConfig() {
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const available = useRuntimeCapability("codebase");
  const { data: role, isLoading: roleLoading } = useEmbeddingRole();
  const { data: status } = useCodebaseStatus(
    available && workspace.status === "ready" ? { cwd } : undefined,
  );
  return {
    cwd,
    status,
    available,
    resolving: workspace.status === "resolving" || roleLoading,
    enabled: workspace.status === "ready" && available && Boolean(role?.model),
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
