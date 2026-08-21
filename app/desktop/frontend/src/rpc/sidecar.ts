import {
  errorMessage,
  parseTransportProblem,
  RpcConnectionError,
  RpcTransportError,
} from "./errors";
import { validateHTTPSidecarResponse } from "@lyra/runtime-contract/validate";
import {
  HTTP_ENDPOINTS,
  type HTTPSidecarEndpointName,
  type HTTPSidecarResponses,
  type LivenessStatus,
  type ReadinessStatus,
  type RuntimeInfo,
} from "@lyra/runtime-contract/wire";

export type { LivenessStatus, ReadinessStatus, RuntimeInfo } from "@lyra/runtime-contract/wire";

export interface SidecarClientConfig {
  baseUrl: string;
  fetch?: typeof fetch;
}

export interface SidecarClient {
  info(signal?: AbortSignal): Promise<RuntimeInfo>;
  liveness(signal?: AbortSignal): Promise<LivenessStatus>;
  readiness(signal?: AbortSignal): Promise<ReadinessStatus>;
}

// A new sidecar in Runtime's generated endpoint set must acquire an SDK method
// in this object or TypeScript fails. The values keep the public method names
// explicit so product-consumer analysis can resolve typed callsites.
const SIDECAR_METHODS = {
  info: "info",
  liveness: "liveness",
  readiness: "readiness",
} as const satisfies Record<HTTPSidecarEndpointName, keyof SidecarClient>;

export function createSidecarClient(config: SidecarClientConfig): SidecarClient {
  const baseUrl = config.baseUrl.replace(/\/+$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  async function getJson<Endpoint extends HTTPSidecarEndpointName>(
    endpoint: Endpoint,
    signal?: AbortSignal,
  ): Promise<HTTPSidecarResponses[Endpoint]> {
    const specification = HTTP_ENDPOINTS[endpoint];
    const path = specification.path;
    let res: Response;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, {
        method: specification.method,
        headers: { Accept: "application/json" },
        signal,
      });
    } catch (err) {
      throw new RpcConnectionError(`sidecar ${path}: ${errorMessage(err)}`);
    }
    let text: string;
    try {
      text = await res.text();
    } catch (err) {
      throw new RpcConnectionError(
        `sidecar ${path}: response could not be read: ${errorMessage(err)}`,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
    if (!specification.responseStatuses.some((status) => status === res.status)) {
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
    let json: unknown;
    try {
      json = JSON.parse(text);
    } catch (err) {
      throw new RpcTransportError(
        `sidecar ${path}: invalid JSON: ${errorMessage(err)}`,
        res.status,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
    const violations = validateHTTPSidecarResponse(endpoint, json);
    if (violations.length > 0) {
      throw new RpcTransportError(
        `sidecar ${path}: response violates its contract: ${violations.map(({ path: field, detail }) => `${field} ${detail}`).join("; ")}`,
        res.status,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
    return json as HTTPSidecarResponses[Endpoint];
  }

  return {
    info: (signal) => getJson(SIDECAR_METHODS.info, signal),
    liveness: (signal) => getJson(SIDECAR_METHODS.liveness, signal),
    readiness: (signal) => getJson(SIDECAR_METHODS.readiness, signal),
  };
}
