import { getContainer } from "@/main/container";
import {
  HTTP_ENDPOINTS,
  type DiscoverResponse,
  type LivenessStatus,
  type ReadinessStatus,
  type RuntimeInfo,
  type ServerCapabilities,
} from "@/rpc";
import type {
  RuntimeConnectionInspector,
  RuntimeServiceHealth,
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

function assertRuntimeIdentity(info: RuntimeInfo, discovery: DiscoverResponse): void {
  if (
    info.server.name !== discovery.serverInfo.name ||
    info.server.version !== discovery.serverInfo.version
  ) {
    throw new Error("Runtime info and discovery identify different servers");
  }
  if (info.protocolVersion !== discovery.protocolVersion) {
    throw new Error("Runtime info and discovery advertise different protocol versions");
  }
}

function assertRuntimeProcessGeneration(
  info: RuntimeInfo,
  liveness: LivenessStatus,
  readiness: ReadinessStatus,
  discovery: DiscoverResponse,
): string {
  const generation = info.server.instanceId;
  if (
    liveness.instanceId !== generation ||
    readiness.instanceId !== generation ||
    discovery.serverInfo.instanceId !== generation
  ) {
    throw new Error("Runtime inspection observed different Runtime process generations");
  }
  return generation;
}

/** Translate typed sidecar responses into Runtime context service facts. */
export function runtimeServiceInspector(): RuntimeConnectionInspector<ServerCapabilities> {
  const sidecar = getContainer().sidecar();
  const client = getContainer().client();
  return {
    async inspect(signal) {
      const cohort = new AbortController();
      const linkedSignal = AbortSignal.any([signal, cohort.signal]);
      let info: RuntimeInfo;
      let liveness: Awaited<ReturnType<typeof sidecar.liveness>>;
      let readiness: ReadinessStatus;
      let discovery: Awaited<ReturnType<typeof client.runtime.discover>>;
      try {
        [info, liveness, readiness, discovery] = await Promise.all([
          sidecar.info(linkedSignal),
          sidecar.liveness(linkedSignal),
          sidecar.readiness(linkedSignal),
          client.runtime.discover(linkedSignal),
        ]);
      } catch (error) {
        cohort.abort();
        throw error;
      }
      assertEndpointIdentity(info);
      assertRuntimeIdentity(info, discovery);
      const processGeneration = assertRuntimeProcessGeneration(
        info,
        liveness,
        readiness,
        discovery,
      );
      if (liveness.status !== "ok") throw new Error("Runtime liveness check failed");

      const checks: RuntimeServiceObservation["checks"] = {};
      for (const [name, health] of Object.entries(readiness.checks ?? {})) {
        checks[name] = serviceHealth(health);
      }
      return {
        processGeneration,
        service: {
          server: { name: info.server.name, version: info.server.version },
          protocolVersion: info.protocolVersion,
          health: serviceHealth(readiness.status),
          checks,
        },
        capabilities: discovery.capabilities,
      };
    },
  };
}
