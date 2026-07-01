import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { t } from "@/lib/i18n";
import {
  errorDetail,
  RpcError,
  type ConfigureProviderRequest,
  type EmbeddingRole,
  type UtilityRole,
} from "@/rpc";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";

// Provider configuration mutations (providers.configure / providers.test).
// Counterpart to the read-side useProviders() query.

export interface SaveProviderInput {
  provider: string;
  apiKey?: string;
  baseUrl?: string;
}

/**
 * Upsert a provider's key / baseUrl (providers.configure) and refetch the
 * providers + models lists so the pane and the composer picker pick up the
 * new enablement immediately.
 */
export function useConfigureProvider(): (input: SaveProviderInput) => Promise<void> {
  return useCallback(async (input) => {
    const params: ConfigureProviderRequest = { provider: input.provider };
    if (input.apiKey) params.apiKey = input.apiKey;
    if (input.baseUrl) params.baseUrl = input.baseUrl;
    await getContainer().client().providers.configure(params);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: [PROVIDERS_KEY] }),
      queryClient.invalidateQueries({ queryKey: [MODELS_KEY] }),
    ]);
  }, []);
}

/**
 * Point the maintenance work (compaction / extraction / titling) at a
 * (provider, model) — an empty model clears it back to the main turn model
 * (models.setUtilityRole). The runtime validates by resolving the client, so
 * an unconfigured provider / unknown model fails server-side; we flatten that
 * to `{ ok:false, error }` here (mirroring useTestProvider) so the pane —
 * which must not import @/rpc — renders the reason inline. On success the
 * utility-role query is refetched so the pane reflects the stored value.
 */
export async function setUtilityRole(role: UtilityRole): Promise<TestOutcome> {
  try {
    await getContainer().client().models.setUtilityRole(role);
    await queryClient.invalidateQueries({ queryKey: [UTILITY_ROLE_KEY] });
    return { ok: true };
  } catch (e) {
    const detail = e instanceof RpcError ? errorDetail(e.data) : undefined;
    return {
      ok: false,
      error: detail ?? (e instanceof Error ? e.message : t("providers.utility.error")),
    };
  }
}

/**
 * Point the @codebase semantic index at an (embedding-capable provider, model)
 * — an empty model clears it (turns the feature off) (models.setEmbeddingRole).
 * Validated server-side (the provider must be embedding-capable + configured);
 * flattened to `{ ok, error }` so the pane renders the reason inline. Refetches
 * the embedding-role + codebase-status queries on success.
 */
export async function setEmbeddingRole(role: EmbeddingRole): Promise<TestOutcome> {
  try {
    await getContainer().client().models.setEmbeddingRole(role);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: [EMBEDDING_ROLE_KEY] }),
      queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] }),
    ]);
    return { ok: true };
  } catch (e) {
    const detail = e instanceof RpcError ? errorDetail(e.data) : undefined;
    return {
      ok: false,
      error: detail ?? (e instanceof Error ? e.message : t("providers.embedding.error")),
    };
  }
}

export interface TestOutcome {
  ok: boolean;
  /** Human-readable failure reason (e.g. a 401 detail), already flattened. */
  error?: string;
}

/**
 * Live-probe a provider (providers.test): the runtime sends a minimal request
 * with the provider's key. A failed probe comes back as `{ ok:false, error }`
 * (NOT an RPC error), so callers render the reason inline.
 */
export function useTestProvider(): (provider: string) => Promise<TestOutcome> {
  return useCallback(async (provider) => {
    const res = await getContainer().client().providers.test(provider);
    return {
      ok: res.ok,
      error: res.ok ? undefined : (errorDetail(res.error) ?? t("providers.error.test")),
    };
  }, []);
}
