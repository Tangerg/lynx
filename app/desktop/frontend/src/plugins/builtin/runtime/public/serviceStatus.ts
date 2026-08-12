import {
  runtimeServiceStatus,
  type RuntimeServiceStatusPort,
} from "../application/ports/serviceStatus";
import { runtimeServiceAcceptsCommands } from "../application/runtimeCommandAvailability";

export type {
  RuntimeServicePhase,
  RuntimeServiceSnapshot,
} from "../application/ports/serviceStatus";

export const useRuntimeServiceStatus: RuntimeServiceStatusPort["useSnapshot"] = () =>
  runtimeServiceStatus().useSnapshot();

/** Whether commands may target the last inspected Runtime connection. */
export function useRuntimeCommandsAvailable(): boolean {
  return runtimeServiceAcceptsCommands(useRuntimeServiceStatus());
}

/** Event-handler form of useRuntimeCommandsAvailable. */
export function runtimeCommandsAvailable(): boolean {
  return runtimeServiceAcceptsCommands(runtimeServiceStatus().snapshot());
}

export function refreshRuntimeServiceStatus(): Promise<void> {
  return runtimeServiceStatus().refresh();
}

/** Re-check the connection after a consumer transport ends, without presenting a manual refresh. */
export function verifyRuntimeServiceConnection(): Promise<void> {
  return runtimeServiceStatus().verify();
}
