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
