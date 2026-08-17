import { create } from "zustand";
import type { ServerCapabilities, WireFeature } from "@/rpc";
import { configureRuntimeCapabilityPort } from "../application/ports/capabilities";
import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "../application/ports/serviceStatus";
import {
  createRuntimeServiceController,
  type RuntimeConnectionInspection,
  type RuntimeConnectionInspector,
  type RuntimeGeneration,
  type RuntimeServiceController,
  type RuntimeServiceFailure,
} from "../application/runtimeService";

interface RuntimeConnectionState {
  generation: RuntimeGeneration | null;
  capabilities: ServerCapabilities | null;
  service: RuntimeServiceSnapshot;
}

function initialConnectionState(): RuntimeConnectionState {
  return {
    generation: null,
    capabilities: null,
    service: { phase: "checking", observation: null, failure: null },
  };
}

export const useRuntimeConnectionStore = create<RuntimeConnectionState>(() =>
  initialConnectionState(),
);

export function useServerFeature(feature: WireFeature): boolean {
  return (
    useRuntimeConnectionStore((state) => state.capabilities?.features[feature]?.enabled) === true
  );
}

function serverFeature(feature: WireFeature): boolean {
  return useRuntimeConnectionStore.getState().capabilities?.features[feature]?.enabled === true;
}

export function runtimeSupportsStreamingMethod(method: string): boolean {
  return (
    useRuntimeConnectionStore.getState().capabilities?.streamingMethods?.includes(method) ?? false
  );
}

export function runtimeSupportsTopic(topic: string): boolean {
  return (
    useRuntimeConnectionStore
      .getState()
      .capabilities?.runtimeTopics.some((advertised) => advertised === topic) ?? false
  );
}

function subscribeRuntimeCapabilities(onChange: () => void): () => void {
  let current = useRuntimeConnectionStore.getState().capabilities;
  return useRuntimeConnectionStore.subscribe((state) => {
    if (state.capabilities === current) return;
    current = state.capabilities;
    onChange();
  });
}

/** Observe the atomic connection identity + capability projection. */
export function subscribeRuntimeConnection(onChange: () => void): () => void {
  let current = useRuntimeConnectionStore.getState();
  return useRuntimeConnectionStore.subscribe((state) => {
    if (state.generation === current.generation && state.capabilities === current.capabilities) {
      current = state;
      return;
    }
    current = state;
    onChange();
  });
}

export function runtimeConnectionGeneration(): RuntimeGeneration | null {
  return useRuntimeConnectionStore.getState().generation;
}

/** Install the read-only capability boundary used by tests without a Runtime owner. */
export function installRuntimeCapabilityPort(): () => void {
  return configureRuntimeCapabilityPort({
    useCapability: useServerFeature,
    hasCapability: serverFeature,
    supportsStreamingMethod: runtimeSupportsStreamingMethod,
    supportsRuntimeTopic: runtimeSupportsTopic,
    negotiated: () => useRuntimeConnectionStore.getState().capabilities,
    subscribe: subscribeRuntimeCapabilities,
  });
}

export interface RuntimeConnectionOwner {
  dispose(): void;
}

let activeConnection: RuntimeConnectionOwnerImplementation | null = null;

class RuntimeConnectionOwnerImplementation implements RuntimeConnectionOwner {
  readonly #controller: RuntimeServiceController;
  readonly #disposeCapabilities: () => void;
  readonly #disposeServiceStatus: () => void;
  #disposed = false;

  constructor(inspector: RuntimeConnectionInspector<ServerCapabilities>) {
    this.#controller = createRuntimeServiceController(inspector, {
      checking: () => this.#checking(),
      replace: (inspection) => this.#replace(inspection),
      unavailable: (failure) => this.#unavailable(failure),
    });
    this.#disposeCapabilities = installRuntimeCapabilityPort();
    this.#disposeServiceStatus = configureRuntimeServiceStatusPort({
      useSnapshot: () => useRuntimeConnectionStore((state) => state.service),
      snapshot: () => useRuntimeConnectionStore.getState().service,
      refresh: () => this.#controller.refresh(),
      verify: () => this.#controller.verify(),
    });
  }

  start(): void {
    if (!this.#ownsGeneration()) return;
    this.#controller.start();
  }

  dispose(): void {
    if (this.#disposed) return;
    const ownsGeneration = activeConnection === this;
    this.#disposed = true;
    this.#controller.dispose();
    try {
      if (!ownsGeneration) return;
      activeConnection = null;
      useRuntimeConnectionStore.setState(initialConnectionState(), true);
    } finally {
      this.#disposeServiceStatus();
      this.#disposeCapabilities();
    }
  }

  #ownsGeneration(): boolean {
    return !this.#disposed && activeConnection === this;
  }

  #checking(): void {
    if (!this.#ownsGeneration()) return;
    useRuntimeConnectionStore.setState((state) => ({
      ...state,
      service: { ...state.service, phase: "checking", failure: null },
    }));
  }

  #replace(inspection: RuntimeConnectionInspection<ServerCapabilities>): void {
    if (!this.#ownsGeneration()) return;
    useRuntimeConnectionStore.setState({
      generation: inspection.generation,
      capabilities: inspection.capabilities,
      service: {
        phase: inspection.service.health,
        observation: inspection.service,
        failure: null,
      },
    });
  }

  #unavailable(failure: RuntimeServiceFailure): void {
    if (!this.#ownsGeneration()) return;
    useRuntimeConnectionStore.setState({
      generation: null,
      capabilities: null,
      service: { phase: "unavailable", observation: null, failure },
    });
  }
}

/**
 * Claim the process-local Runtime connection generation. The claim retires the
 * previous controller before publishing an empty successor projection, so old
 * inspections, timers, and disposers can no longer write or clear current state.
 */
export function startRuntimeConnection(
  inspector: RuntimeConnectionInspector<ServerCapabilities>,
): RuntimeConnectionOwner {
  const retired = activeConnection;
  const owner = new RuntimeConnectionOwnerImplementation(inspector);
  activeConnection = owner;
  retired?.dispose();
  useRuntimeConnectionStore.setState(initialConnectionState(), true);
  owner.start();
  return owner;
}

export function resetRuntimeConnectionForTest(): void {
  activeConnection?.dispose();
  activeConnection = null;
  useRuntimeConnectionStore.setState(initialConnectionState(), true);
}
