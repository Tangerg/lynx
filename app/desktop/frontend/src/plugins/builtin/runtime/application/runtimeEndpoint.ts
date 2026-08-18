import { z } from "zod";
import { configuredRuntimeEndpoint, runtimeEndpointConfiguration } from "./ports/runtimeEndpoint";

export const DEFAULT_RUNTIME_ENDPOINT = "http://127.0.0.1:17171";

const UrlSchema = z.url();

export type RuntimeEndpointRejection = "invalid_url" | "unsupported_scheme";

export type RuntimeEndpointChange =
  | {
      kind: "applied";
      endpoint: string;
      changed: boolean;
    }
  | {
      kind: "rejected";
      input: string;
      reason: RuntimeEndpointRejection;
    };

function acceptedEndpoint(input: string): string | null {
  const result = UrlSchema.safeParse(input);
  if (!result.success) return null;
  const protocol = new URL(result.data).protocol;
  return protocol === "http:" || protocol === "https:" ? result.data : null;
}

export function currentRuntimeEndpoint(): string {
  const configured = configuredRuntimeEndpoint()?.read()?.trim();
  if (!configured) return DEFAULT_RUNTIME_ENDPOINT;
  return acceptedEndpoint(configured) ?? DEFAULT_RUNTIME_ENDPOINT;
}

export function applyRuntimeEndpoint(input: string): RuntimeEndpointChange {
  const configuration = runtimeEndpointConfiguration();
  const current = currentRuntimeEndpoint();
  const trimmed = input.trim();
  if (!trimmed) {
    const changed = current !== DEFAULT_RUNTIME_ENDPOINT;
    if (changed) configuration.replace(DEFAULT_RUNTIME_ENDPOINT);
    return {
      kind: "applied",
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
      changed,
    };
  }

  const parsed = UrlSchema.safeParse(trimmed);
  if (!parsed.success) {
    return { kind: "rejected", input, reason: "invalid_url" };
  }

  const protocol = new URL(parsed.data).protocol;
  if (protocol !== "http:" && protocol !== "https:") {
    return { kind: "rejected", input, reason: "unsupported_scheme" };
  }

  const changed = current !== parsed.data;
  if (changed) configuration.replace(parsed.data);
  return { kind: "applied", endpoint: parsed.data, changed };
}

export function resetRuntimeEndpoint(): RuntimeEndpointChange {
  return applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT);
}
