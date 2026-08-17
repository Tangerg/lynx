import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { RuntimeServiceFailure, RuntimeServiceObservation } from "../runtimeService";

export type RuntimeServicePhase =
  "checking" | "reconnecting" | "ready" | "degraded" | "unhealthy" | "unavailable";

export interface RuntimeServiceSnapshot {
  phase: RuntimeServicePhase;
  observation: RuntimeServiceObservation | null;
  failure: RuntimeServiceFailure | null;
}

export interface RuntimeServiceStatusPort {
  useSnapshot(): RuntimeServiceSnapshot;
  snapshot(): RuntimeServiceSnapshot;
  refresh(): Promise<void>;
}

const port = createSingletonPort<RuntimeServiceStatusPort>(
  "Runtime service status port is not configured",
);

export const configureRuntimeServiceStatusPort = port.configure;
export const runtimeServiceStatus = port.get;
