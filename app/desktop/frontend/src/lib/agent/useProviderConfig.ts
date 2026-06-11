import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getContainer } from "@/main/container";
import { errorDetail, type ConfigureProviderRequest } from "@/rpc";
import { MODELS_KEY, PROVIDERS_KEY } from "@/lib/data/queries";

// Provider configuration mutations (providers.configure / providers.test).
// Lives in lib/ so the settings pane (a component) reaches the runtime through
// a hook rather than importing @/rpc / @/main directly (layer rule). Counterpart
// to the read-side useProviders() query.

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
  const queryClient = useQueryClient();
  return useCallback(
    async (input) => {
      const params: ConfigureProviderRequest = { provider: input.provider };
      if (input.apiKey) params.apiKey = input.apiKey;
      if (input.baseUrl) params.baseUrl = input.baseUrl;
      await getContainer().client().providers.configure(params);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: [PROVIDERS_KEY] }),
        queryClient.invalidateQueries({ queryKey: [MODELS_KEY] }),
      ]);
    },
    [queryClient],
  );
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
    return { ok: res.ok, error: res.ok ? undefined : (errorDetail(res.error) ?? "Test failed") };
  }, []);
}
