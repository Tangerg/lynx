import { useCallback } from "react";
import { t } from "@/lib/i18n";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  type ProviderInfo,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  useEmbeddingRole,
  useModels,
  useProviders,
  useUtilityRole,
} from "./providerQueries";
import { queryClient } from "@/lib/data/queryClient";
import {
  providerGateway,
  type ProviderCredentials,
  type ProviderRole,
} from "./ports/providerGateway";

// Provider configuration mutations (providers.configure / providers.test).
// Counterpart to the read-side useProviders() query.

export type ProviderConfig = ProviderInfo;

export function useProviderConfigs() {
  return useProviders();
}

export function useProviderRoleConfig() {
  const utilityRole = useUtilityRole();
  const embeddingRole = useEmbeddingRole();
  const models = useModels();
  const providers = useProviders();
  return { utilityRole, embeddingRole, models, providers };
}

export function useUtilityModelConfig() {
  const { utilityRole, models } = useProviderRoleConfig();
  const role = utilityRole.data;
  const modelOptions = models.data ?? [];
  const selected =
    role?.provider && role.model
      ? (modelOptions.find(
          (model) => model.provider === role.provider && model.id === role.model,
        ) ?? null)
      : null;
  return {
    role,
    modelOptions,
    selected,
    isSet: Boolean(role?.model),
  };
}

export function useEmbeddingModelConfig() {
  const { embeddingRole, providers } = useProviderRoleConfig();
  const role = embeddingRole.data;
  const providerConfigs = providers.data ?? [];
  return {
    role,
    providers: providerConfigs,
    capableProviders: providerConfigs.filter((provider) => provider.embeddingCapable),
    isSet: Boolean(role?.model),
  };
}

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
    const params: ProviderCredentials = { provider: input.provider };
    if (input.apiKey) params.apiKey = input.apiKey;
    if (input.baseUrl) params.baseUrl = input.baseUrl;
    await providerGateway().configureProvider(params);
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
export async function setUtilityRole(role: ProviderRole): Promise<TestOutcome> {
  try {
    await providerGateway().setUtilityRole(role);
    await queryClient.invalidateQueries({ queryKey: [UTILITY_ROLE_KEY] });
    return { ok: true };
  } catch (e) {
    const detail = providerGateway().errorMessage(e);
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
export async function setEmbeddingRole(role: ProviderRole): Promise<TestOutcome> {
  try {
    await providerGateway().setEmbeddingRole(role);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: [EMBEDDING_ROLE_KEY] }),
      queryClient.invalidateQueries({ queryKey: [CODEBASE_STATUS_KEY] }),
    ]);
    return { ok: true };
  } catch (e) {
    const detail = providerGateway().errorMessage(e);
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
    const res = await providerGateway().testProvider(provider);
    return {
      ok: res.ok,
      error: res.ok ? undefined : (res.error ?? t("providers.error.test")),
    };
  }, []);
}
