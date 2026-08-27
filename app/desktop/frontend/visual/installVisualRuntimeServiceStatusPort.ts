import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "@/plugins/builtin/runtime/application/ports/serviceStatus";

/**
 * A connected Runtime, stated rather than probed.
 *
 * Production UI asks whether commands may be sent. A visual fixture must answer
 * through the same port without depending on a live process or health probe.
 * This one frozen observation keeps every golden deterministic by construction.
 */
export function installVisualRuntimeServiceStatusPort(): void {
  const snapshot = {
    phase: "ready",
    observation: {
      server: { name: "scopeapp-runtime", version: "0.0.0-visual" },
      protocolVersion: "2",
      health: "ready",
      checks: {},
    },
    failure: null,
  } as const satisfies RuntimeServiceSnapshot;

  configureRuntimeServiceStatusPort({
    useSnapshot: () => snapshot,
    snapshot: () => snapshot,
    refresh: () => Promise.resolve(),
  });
}
