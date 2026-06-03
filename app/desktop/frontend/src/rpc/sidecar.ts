// Sidecar HTTP endpoints — flat JSON, **not** JSON-RPC envelope.
// See TRANSPORT.md §12. Used for liveness probes + version negotiation
// before the JSON-RPC handshake. No Bearer token, no Last-Event-Id,
// no envelope — curl-friendly metadata only.
//
// These are HTTP-transport-only — InProcess/Wails IPC don't have an
// equivalent (those cover the same need via runtime.initialize /
// runtime.ping). `/v2/info` content == the initialize response's flat
// subset; `/v2/health` == runtime.ping (TRANSPORT.md §12).

import { RpcTransportError } from "./errors";
import type { ServerCapabilities, ServerInfo } from "./shapes";

/**
 * Response of `GET /v2/info` — the flat subset of the `runtime.initialize`
 * result, accessible pre-handshake. Backed by the same server state as
 * `runtime.initialize` (TRANSPORT.md §12.2).
 */
export interface RuntimeInfo {
  protocolVersion: string;
  serverInfo: ServerInfo;
  capabilities: ServerCapabilities;
}

export interface HealthStatus {
  status: "ok" | "degraded" | "unhealthy";
  checks?: Record<string, "ok" | "degraded" | "unhealthy">;
}

export interface SidecarClientConfig {
  baseUrl: string;
  fetch?: typeof fetch;
}

export interface SidecarClient {
  info(signal?: AbortSignal): Promise<RuntimeInfo>;
  health(signal?: AbortSignal): Promise<HealthStatus>;
}

export function createSidecarClient(config: SidecarClientConfig): SidecarClient {
  const baseUrl = config.baseUrl.replace(/\/$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  async function getJson<T>(path: string, signal?: AbortSignal): Promise<T> {
    let res: Response;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, {
        method: "GET",
        headers: { Accept: "application/json" },
        signal,
      });
    } catch (err) {
      throw new RpcTransportError(`sidecar ${path}: ${(err as Error).message}`);
    }
    // 503 from /v2/health is still valid JSON — let caller see `status` field.
    if (!res.ok && res.status !== 503) {
      throw new RpcTransportError(`sidecar ${path}: http ${res.status}`, res.status);
    }
    const text = await res.text();
    try {
      return JSON.parse(text) as T;
    } catch (err) {
      throw new RpcTransportError(`sidecar ${path}: invalid JSON: ${(err as Error).message}`);
    }
  }

  return {
    info: (signal) => getJson<RuntimeInfo>("/v2/info", signal),
    health: (signal) => getJson<HealthStatus>("/v2/health", signal),
  };
}
