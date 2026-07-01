import { z } from "zod";
import { RUNTIME_BASE } from "@/main/config";
import { t } from "@/lib/i18n";
import { getConfig, setConfig } from "@/plugins/sdk";

export const RUNTIME_BASE_URL = RUNTIME_BASE;
export const RUNTIME_BASE_CONFIG_KEY = "api.baseUrl";
export const RUNTIME_BASE_STORAGE_KEY = "api.baseUrl";

const UrlSchema = z
  .url()
  .refine((value) => value.startsWith("http://") || value.startsWith("https://"), {
    message: t("connection.error.urlScheme"),
  });

export interface RuntimeBaseUrlResult {
  url: string;
  error: string | null;
}

export function currentRuntimeBaseUrl(): string {
  return (getConfig<string>(RUNTIME_BASE_CONFIG_KEY) ?? RUNTIME_BASE_URL) || RUNTIME_BASE_URL;
}

export function applyRuntimeBaseUrl(input: string): RuntimeBaseUrlResult {
  const trimmed = input.trim();
  if (!trimmed) {
    setConfig(RUNTIME_BASE_CONFIG_KEY, RUNTIME_BASE_URL);
    return { url: RUNTIME_BASE_URL, error: null };
  }
  const result = UrlSchema.safeParse(trimmed);
  if (!result.success) {
    return {
      url: input,
      error: result.error.issues[0]?.message ?? t("connection.error.invalidUrl"),
    };
  }
  setConfig(RUNTIME_BASE_CONFIG_KEY, result.data);
  return { url: result.data, error: null };
}

export function resetRuntimeBaseUrl(): string {
  setConfig(RUNTIME_BASE_CONFIG_KEY, RUNTIME_BASE_URL);
  return RUNTIME_BASE_URL;
}
