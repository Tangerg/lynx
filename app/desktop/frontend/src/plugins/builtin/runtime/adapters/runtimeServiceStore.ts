import { create } from "zustand";
import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "../application/ports/serviceStatus";
import type {
  RuntimeServiceController,
  RuntimeServiceFailure,
  RuntimeServiceObservation,
} from "../application/runtimeService";

interface RuntimeServiceState {
  snapshot: RuntimeServiceSnapshot;
  checking(): void;
  replace(observation: RuntimeServiceObservation): void;
  unavailable(failure: RuntimeServiceFailure): void;
  clear(): void;
}

const initial: RuntimeServiceSnapshot = {
  phase: "checking",
  observation: null,
  failure: null,
};

export const useRuntimeServiceStore = create<RuntimeServiceState>((set) => ({
  snapshot: initial,
  checking: () =>
    set((state) => ({
      snapshot: { ...state.snapshot, phase: "checking", failure: null },
    })),
  replace: (observation) =>
    set({ snapshot: { phase: observation.health, observation, failure: null } }),
  unavailable: (failure) => set({ snapshot: { phase: "unavailable", observation: null, failure } }),
  clear: () => set({ snapshot: initial }),
}));

export function installRuntimeServiceStatusPort(controller: RuntimeServiceController): () => void {
  return configureRuntimeServiceStatusPort({
    useSnapshot: () => useRuntimeServiceStore((state) => state.snapshot),
    snapshot: () => useRuntimeServiceStore.getState().snapshot,
    refresh: () => controller.refresh(),
  });
}
