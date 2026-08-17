import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";

export type { ProviderConfiguration, ProviderRole } from "./providerModels";

export interface CodebaseStatusReadModel {
  state: "none" | "indexing" | "ready" | "error";
  modelId?: string;
  fileCount: number;
  chunkCount: number;
  indexedAt?: string;
  truncated?: boolean;
  operationId?: string;
}

export interface SelectableModel {
  id: string;
  provider: string;
  label: string;
  multimodal: boolean;
  contextWindow?: number;
}

export interface CodebaseStatusQuery {
  cwd?: string;
}

/**
 * A stored role is configuration intent, not proof that its provider is usable
 * right now. Stored credentials can be cleared (or an environment credential
 * can disappear between launches) without erasing the role. Join the role with
 * providers.list whenever a feature needs the effective availability.
 */
export function providerRoleIsAvailable(
  role: ProviderRole | undefined,
  providers: readonly ProviderConfiguration[],
): boolean {
  return Boolean(
    role?.provider &&
    role.model &&
    providers.some((provider) => provider.id === role.provider && provider.apiKeyMasked !== ""),
  );
}

export const PROVIDERS_KEY = "providers";
export const MODELS_KEY = "models";
export const UTILITY_ROLE_KEY = "utility-role";
export const EMBEDDING_ROLE_KEY = "embedding-role";
export const CODEBASE_STATUS_KEY = "codebase-status";

export function commitCodebaseReindexStarted(
  query: CodebaseStatusQuery,
  operationId: string,
): void {
  queryClient.setQueryData<CodebaseStatusReadModel>([CODEBASE_STATUS_KEY, query], (current) => ({
    ...current,
    state: "indexing",
    fileCount: current?.fileCount ?? 0,
    chunkCount: current?.chunkCount ?? 0,
    operationId,
  }));
}

const ACTIVE_CODEBASE_STATUS_REFRESH_MS = 1_000;

// Runtime owns background reindex execution and exposes operationId as its
// liveness handle. Refresh only while that handle exists, then let the final
// ready/error status return this query to its normal cache lifetime.
export function codebaseStatusRefreshInterval(
  status: CodebaseStatusReadModel | undefined,
): number | false {
  return status?.operationId ? ACTIVE_CODEBASE_STATUS_REFRESH_MS : false;
}

export const useModels = createDataQuery<SelectableModel[]>(MODELS_KEY);
export const useProviders = createDataQuery<ProviderConfiguration[]>(PROVIDERS_KEY);
export const useUtilityRole = createDataQuery<ProviderRole>(UTILITY_ROLE_KEY);
export const useEmbeddingRole = createDataQuery<ProviderRole>(EMBEDDING_ROLE_KEY);
export const useCodebaseStatus = createParameterizedDataQuery<
  CodebaseStatusQuery,
  CodebaseStatusReadModel
>(CODEBASE_STATUS_KEY, { refetchInterval: codebaseStatusRefreshInterval });
