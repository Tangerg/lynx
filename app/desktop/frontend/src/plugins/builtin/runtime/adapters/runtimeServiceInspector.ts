import { getContainer } from "@/main/container";
import { HTTP_ENDPOINTS, type ReadinessStatus, type RuntimeInfo } from "@/rpc";
import type {
  RuntimeServiceHealth,
  RuntimeServiceInspector,
  RuntimeServiceObservation,
} from "../application/runtimeService";

function serviceHealth(status: ReadinessStatus["status"]): RuntimeServiceHealth {
  return status === "ok" ? "ready" : status;
}

function assertEndpointIdentity(info: RuntimeInfo): void {
  const expected = {
    rpc: HTTP_ENDPOINTS.rpc.path,
    info: HTTP_ENDPOINTS.info.path,
    liveness: HTTP_ENDPOINTS.liveness.path,
    readiness: HTTP_ENDPOINTS.readiness.path,
  };
  for (const [name, path] of Object.entries(expected)) {
    if (info.endpoints[name as keyof RuntimeInfo["endpoints"]] !== path) {
      throw new Error(`Runtime advertises an incompatible ${name} endpoint`);
    }
  }
}

/** Translate typed sidecar responses into Runtime context service facts. */
export function runtimeServiceInspector(): RuntimeServiceInspector {
  const client = getContainer().sidecar();
  return {
    async inspect(signal) {
      const cohort = new AbortController();
      const linkedSignal = AbortSignal.any([signal, cohort.signal]);
      let info: RuntimeInfo;
      let liveness: Awaited<ReturnType<typeof client.liveness>>;
      let readiness: ReadinessStatus;
      try {
        [info, liveness, readiness] = await Promise.all([
          client.info(linkedSignal),
          client.liveness(linkedSignal),
          client.readiness(linkedSignal),
        ]);
      } catch (error) {
        cohort.abort();
        throw error;
      }
      assertEndpointIdentity(info);
      if (liveness.status !== "ok") throw new Error("Runtime liveness check failed");

      const checks: RuntimeServiceObservation["checks"] = {};
      for (const [name, health] of Object.entries(readiness.checks ?? {})) {
        checks[name] = serviceHealth(health);
      }
      return {
        server: { name: info.server.name, version: info.server.version },
        protocol: {
          current: info.protocol.current,
          minSupported: info.protocol.minSupported,
        },
        health: serviceHealth(readiness.status),
        checks,
      };
    },
  };
}
