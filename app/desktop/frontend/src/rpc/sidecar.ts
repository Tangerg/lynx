import { z } from "zod";
import { errorMessage, parseTransportProblem, RpcTransportError } from "./errors";

const INFO_PATH = "/v2/info";
const LIVENESS_PATH = "/v2/health/live";
const READINESS_PATH = "/v2/health/ready";

const HealthStateSchema = z.enum(["ok", "degraded", "unhealthy"]);

const RuntimeInfoSchema = z.looseObject({
  protocol: z.looseObject({
    current: z.string(),
    minSupported: z.string(),
  }),
  server: z.looseObject({
    name: z.string(),
    version: z.string(),
  }),
  transport: z.literal("http"),
  endpoints: z.looseObject({
    rpc: z.string(),
    info: z.string(),
    liveness: z.string(),
    readiness: z.string(),
  }),
});

const LivenessStatusSchema = z.looseObject({ status: z.literal("ok") });

const ReadinessStatusSchema = z.looseObject({
  status: HealthStateSchema,
  checks: z.record(z.string(), HealthStateSchema).optional(),
});

export type RuntimeInfo = z.infer<typeof RuntimeInfoSchema>;
export type LivenessStatus = z.infer<typeof LivenessStatusSchema>;
export type ReadinessStatus = z.infer<typeof ReadinessStatusSchema>;

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
    schema: z.ZodType<T>,
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
      throw new RpcTransportError(`sidecar ${path}: ${errorMessage(err)}`);
    }
    let text: string;
    try {
      text = await res.text();
    } catch (err) {
      throw new RpcTransportError(
        `sidecar ${path}: response could not be read: ${errorMessage(err)}`,
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
    const parsed = schema.safeParse(json);
    if (!parsed.success) {
      throw new RpcTransportError(
        `sidecar ${path}: response violates its contract: ${parsed.error.message}`,
        res.status,
        res.headers.get("Request-Id") ?? undefined,
      );
    }
    return parsed.data;
  }

  return {
    info: (signal) => getJson(INFO_PATH, RuntimeInfoSchema, signal),
    liveness: (signal) => getJson(LIVENESS_PATH, LivenessStatusSchema, signal),
    readiness: (signal) => getJson(READINESS_PATH, ReadinessStatusSchema, signal, true),
  };
}
