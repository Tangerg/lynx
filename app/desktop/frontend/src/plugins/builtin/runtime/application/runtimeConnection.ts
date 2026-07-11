import { z } from "zod";
import { RUNTIME_BASE, RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import { t } from "@/lib/i18n";
import { getConfig, setConfig, type Host } from "@/plugins/sdk";

export const DEFAULT_RUNTIME_ENDPOINT = RUNTIME_BASE;
const RUNTIME_ENDPOINT_STORAGE_KEY = "endpoint";

const UrlSchema = z
  .url()
  .refine((value) => value.startsWith("http://") || value.startsWith("https://"), {
    message: t("connection.error.urlScheme"),
  });

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

// Host config is in-memory; mirror the runtime URL through plugin storage so
// the first RPC client built after launch sees the persisted endpoint.
export function installRuntimeConnection(host: Pick<Host, "config" | "storage">): void {
  const stored = host.storage.get<string>(RUNTIME_ENDPOINT_STORAGE_KEY);
  if (typeof stored === "string" && stored) {
    host.config.set(RUNTIME_ENDPOINT_CONFIG_KEY, stored);
  }

  host.config.onChange(RUNTIME_ENDPOINT_CONFIG_KEY, (value) => {
    if (typeof value === "string") host.storage.set(RUNTIME_ENDPOINT_STORAGE_KEY, value);
  });
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
    return {
      endpoint: input,
      error: result.error.issues[0]?.message ?? t("connection.error.invalidUrl"),
      changed: false,
    };
  }
  setConfig(RUNTIME_ENDPOINT_CONFIG_KEY, result.data);
  return { endpoint: result.data, error: null, changed: current !== result.data };
}

export function resetRuntimeEndpoint(): RuntimeEndpointResult {
  return applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT);
}
