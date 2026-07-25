import { z } from "zod";
import { RUNTIME_BASE, RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import { t } from "@/lib/i18n";
import { getConfig, setConfig } from "@/plugins/sdk";

export const DEFAULT_RUNTIME_ENDPOINT = RUNTIME_BASE;

// A validator, not a message: `t()` here would run at module load and bake in
// whatever locale was active then, so switching language left this one string
// behind. The words are chosen where the user submits.
const UrlSchema = z.url();
const isHttpUrl = (value: string): boolean =>
  value.startsWith("http://") || value.startsWith("https://");

export interface RuntimeEndpointResult {
  endpoint: string;
  error: string | null;
  changed: boolean;
}

export function currentRuntimeEndpoint(): string {
  return (
    (getConfig<string>(RUNTIME_ENDPOINT_CONFIG_KEY) ?? DEFAULT_RUNTIME_ENDPOINT) ||
    DEFAULT_RUNTIME_ENDPOINT
  );
}

export function applyRuntimeEndpoint(input: string): RuntimeEndpointResult {
  const current = currentRuntimeEndpoint();
  const trimmed = input.trim();
  if (!trimmed) {
    setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, DEFAULT_RUNTIME_ENDPOINT);
    return {
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
      error: null,
      changed: current !== DEFAULT_RUNTIME_ENDPOINT,
    };
  }
  const result = UrlSchema.safeParse(trimmed);
  if (!result.success) {
    return { endpoint: input, error: t("connection.error.invalidUrl"), changed: false };
  }
  if (!isHttpUrl(result.data)) {
    return { endpoint: input, error: t("connection.error.urlScheme"), changed: false };
  }
  setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, result.data);
  return { endpoint: result.data, error: null, changed: current !== result.data };
}

export function resetRuntimeEndpoint(): RuntimeEndpointResult {
  return applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT);
}
