import { create } from "zustand";
import { createPublicationSlot } from "@/lib/publicationSlot";
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
  type RuntimeProcessGeneration,
  type RuntimeServiceController,
  type RuntimeServiceFailure,
} from "../application/runtimeService";

/** Process-local capability identifying one admitted client/server connection.
 *  A reconnect to the same Runtime process still creates a successor identity. */
export type RuntimeConnectionGeneration = string;

interface RuntimeConnectionState {
  connectionGeneration: RuntimeConnectionGeneration | null;
  processGeneration: RuntimeProcessGeneration | null;
  capabilities: ServerCapabilities | null;
  service: RuntimeServiceSnapshot;
}

function initialConnectionState(): RuntimeConnectionState {
  return {
    connectionGeneration: null,
    processGeneration: null,
    capabilities: null,
    service: { phase: "checking", observation: null, failure: null },
  };
}

function reconnectingConnectionState(): RuntimeConnectionState {
  return {
    connectionGeneration: null,
    processGeneration: null,
    capabilities: null,
    service: { phase: "reconnecting", observation: null, failure: null },
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

/** Install the read-only capability boundary used by tests without a Runtime owner. */
export function installRuntimeCapabilityPort(): () => void {
  return configureRuntimeCapabilityPort({
    useCapability: useServerFeature,
    hasCapability: serverFeature,
    supportsStreamingMethod: runtimeSupportsStreamingMethod,
    supportsRuntimeTopic: runtimeSupportsTopic,
    negotiated: () => useRuntimeConnectionStore.getState().capabilities,
  });
}

export interface RuntimeConnectionOwner {
  connectionGeneration(): RuntimeConnectionGeneration | null;
  subscribeConnection(onChange: () => void): () => void;
  subscribeServerReplacement(onReplace: () => void): () => void;
  replaceEndpoint(commit: () => void): Promise<void>;
  reportConnectionLoss(expectedGeneration: RuntimeConnectionGeneration): Promise<void>;
  dispose(): void;
}

let connectionGenerationSequence = 0;

function nextConnectionGeneration(processGeneration: RuntimeProcessGeneration): string {
  connectionGenerationSequence += 1;
  return `${processGeneration}:${connectionGenerationSequence}`;
}

class RuntimeConnectionOwnerImplementation implements RuntimeConnectionOwner {
  readonly #controller: RuntimeServiceController;
  readonly #disposeCapabilities: () => void;
  readonly #disposeServiceStatus: () => void;
  readonly #serverReplacementListeners = new Set<() => void>();
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
    });
  }

  start(): void {
    if (!this.#ownsGeneration()) return;
    this.#controller.start();
  }

  connectionGeneration(): RuntimeConnectionGeneration | null {
    return this.#ownsGeneration()
      ? useRuntimeConnectionStore.getState().connectionGeneration
      : null;
  }

  subscribeConnection(onChange: () => void): () => void {
    if (!this.#ownsGeneration()) return () => undefined;
    let current = useRuntimeConnectionStore.getState();
    return useRuntimeConnectionStore.subscribe((state) => {
      if (!this.#ownsGeneration()) return;
      if (
        state.connectionGeneration === current.connectionGeneration &&
        state.capabilities === current.capabilities
      ) {
        current = state;
        return;
      }
      current = state;
      onChange();
    });
  }

  subscribeServerReplacement(onReplace: () => void): () => void {
    if (!this.#ownsGeneration()) return () => undefined;
    this.#serverReplacementListeners.add(onReplace);
    return () => this.#serverReplacementListeners.delete(onReplace);
  }

  dispose(): void {
    if (this.#disposed) return;
    const ownsGeneration = runtimeConnectionPublication.owns(this);
    this.#disposed = true;
    this.#controller.dispose();
    this.#serverReplacementListeners.clear();
    try {
      if (!ownsGeneration) return;
      runtimeConnectionPublication.withdraw(this);
      useRuntimeConnectionStore.setState(initialConnectionState(), true);
    } finally {
      this.#disposeServiceStatus();
      this.#disposeCapabilities();
    }
  }

  replaceEndpoint(commit: () => void): Promise<void> {
    if (!this.#ownsGeneration()) return Promise.resolve();
    // Server scope is broader than a transport reconnect. Revoke the old
    // connection first, commit the endpoint only after every synchronous
    // connection subscriber has retired its writers, then let read-model
    // owners discard facts that cannot cross server identity.
    useRuntimeConnectionStore.setState(initialConnectionState(), true);
    commit();
    for (const listener of this.#serverReplacementListeners) listener();
    return this.#controller.recover();
  }

  reportConnectionLoss(expectedGeneration: RuntimeConnectionGeneration): Promise<void> {
    if (!this.#ownsGeneration()) return Promise.resolve();
    const current = useRuntimeConnectionStore.getState();
    if (current.connectionGeneration !== expectedGeneration) return Promise.resolve();

    // The stream is an ordered member of this connection generation. Once it
    // ends unexpectedly, the generation is no longer capable of admitting
    // commands, queries, mutations, or material writers — even if the same
    // Runtime process will answer the recovery inspection a moment later.
    useRuntimeConnectionStore.setState(reconnectingConnectionState(), true);
    return this.#controller.recover();
  }

  #ownsGeneration(): boolean {
    return !this.#disposed && runtimeConnectionPublication.owns(this);
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
    const current = useRuntimeConnectionStore.getState();
    const connectionGeneration =
      current.connectionGeneration !== null &&
      current.processGeneration === inspection.processGeneration
        ? current.connectionGeneration
        : nextConnectionGeneration(inspection.processGeneration);
    useRuntimeConnectionStore.setState({
      connectionGeneration,
      processGeneration: inspection.processGeneration,
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
      connectionGeneration: null,
      processGeneration: null,
      capabilities: null,
      service: { phase: "unavailable", observation: null, failure },
    });
  }
}

const runtimeConnectionPublication = createPublicationSlot<RuntimeConnectionOwnerImplementation>();

/**
 * Claim the process-local Runtime connection owner. The claim retires the
 * previous controller before publishing an empty successor projection, so old
 * inspections, timers, and disposers can no longer write or clear current state.
 */
export function startRuntimeConnection(
  inspector: RuntimeConnectionInspector<ServerCapabilities>,
): RuntimeConnectionOwner {
  const owner = new RuntimeConnectionOwnerImplementation(inspector);
  runtimeConnectionPublication.publish(owner, (predecessor) => predecessor.dispose());
  useRuntimeConnectionStore.setState(initialConnectionState(), true);
  owner.start();
  return owner;
}

export function resetRuntimeConnectionForTest(): void {
  runtimeConnectionPublication.current()?.dispose();
  useRuntimeConnectionStore.setState(initialConnectionState(), true);
}
