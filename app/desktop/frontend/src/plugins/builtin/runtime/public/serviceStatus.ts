import {
  runtimeServiceStatus,
  type RuntimeServiceStatusPort,
} from "../application/ports/serviceStatus";

export type {
  RuntimeServicePhase,
  RuntimeServiceSnapshot,
} from "../application/ports/serviceStatus";

export const useRuntimeServiceStatus: RuntimeServiceStatusPort["useSnapshot"] = () =>
  runtimeServiceStatus().useSnapshot();

export function refreshRuntimeServiceStatus(): Promise<void> {
  return runtimeServiceStatus().refresh();
}

/** Re-check the connection after a consumer transport ends, without presenting a manual refresh. */
export function verifyRuntimeServiceConnection(): Promise<void> {
  return runtimeServiceStatus().verify();
}
