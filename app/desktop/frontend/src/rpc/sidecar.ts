import { parseTransportProblem, RpcTransportError } from "./errors";

const INFO_PATH = "/v2/info";
const LIVENESS_PATH = "/v2/health/live";
const READINESS_PATH = "/v2/health/ready";

export interface RuntimeInfo {
  protocol: {
    current: string;
    minSupported: string;
  };
  server: {
    name: string;
    version: string;
  };
  transport: "http";
  endpoints: {
    rpc: string;
    info: string;
    liveness: string;
    readiness: string;
  };
}

export interface LivenessStatus {
  status: "ok";
}

export interface ReadinessStatus {
  status: "ok" | "degraded" | "unhealthy";
  checks?: Record<string, "ok" | "degraded" | "unhealthy">;
}

export interface SidecarClientConfig {
  baseUrl: string;
  fetch?: typeof fetch;
}

export interface SidecarClient {
  info(signal?: AbortSignal): Promise<RuntimeInfo>;
  liveness(signal?: AbortSignal): Promise<LivenessStatus>;
  readiness(signal?: AbortSignal): Promise<ReadinessStatus>;
}

export function createSidecarClient(config: SidecarClientConfig): SidecarClient {
  const baseUrl = config.baseUrl.replace(/\/+$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  async function getJson<T>(
    path: string,
    signal?: AbortSignal,
    acceptUnavailable = false,
  ): Promise<T> {
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
    let text: string;
    try {
      text = await res.text();
    } catch (err) {
      throw new RpcTransportError(
        `sidecar ${path}: response could not be read: ${(err as Error).message}`,
        res.status,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
    if (!res.ok && !(acceptUnavailable && res.status === 503)) {
      const problem = parseTransportProblem(text);
      const requestId = problem?.requestId ?? res.headers.get("Request-Id") ?? undefined;
      const detail = problem?.detail || res.statusText || "sidecar request failed";
      throw new RpcTransportError(
        `sidecar ${path}: http ${res.status}: ${detail}`,
        res.status,
        requestId,
        problem?.type,
      );
    }
    try {
      return JSON.parse(text) as T;
    } catch (err) {
      throw new RpcTransportError(
        `sidecar ${path}: invalid JSON: ${(err as Error).message}`,
        res.status,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
  }

  return {
    info: (signal) => getJson<RuntimeInfo>(INFO_PATH, signal),
    liveness: (signal) => getJson<LivenessStatus>(LIVENESS_PATH, signal),
    readiness: (signal) => getJson<ReadinessStatus>(READINESS_PATH, signal, true),
  };
}
