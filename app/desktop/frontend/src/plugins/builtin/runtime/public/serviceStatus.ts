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
