import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  providerRoleIsAvailable,
  useCodebaseStatus,
  useEmbeddingRole,
  useProviders,
} from "@/plugins/builtin/settings/providers/public/queries";
import type { CodebaseReindexOperation, CodebaseSearchHit } from "./ports/codebaseGateway";
import { CodebaseCommandOwner, codebaseCommandWasRetired } from "./codebaseCommandOwner";

export type { CodebaseSearchHit } from "./ports/codebaseGateway";

export function useCodebaseSearchConfig() {
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const available = useRuntimeCapability("codebase");
  const { data: role, isLoading: roleLoading } = useEmbeddingRole();
  const { data: providers, isLoading: providersLoading } = useProviders();
  const roleAvailable = providerRoleIsAvailable(role, providers ?? []);
  const { data: status } = useCodebaseStatus(
    available && workspace.status === "ready" ? { cwd } : undefined,
  );
  return {
    cwd,
    status,
    available,
    resolving: workspace.status === "resolving" || roleLoading || providersLoading,
    enabled: workspace.status === "ready" && available && roleAvailable,
  };
}

export async function searchCodebase(
  cwd: string | undefined,
  query: string,
  limit = 12,
): Promise<CodebaseSearchHit[]> {
  return CodebaseCommandOwner.current().search(cwd, query, limit);
}

export async function reindexCodebase(cwd: string | undefined): Promise<CodebaseReindexOperation> {
  return CodebaseCommandOwner.current().reindex(cwd);
}

export { codebaseCommandWasRetired };
