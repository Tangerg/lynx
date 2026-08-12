export type RuntimeServiceHealth = "ready" | "degraded" | "unhealthy";

export interface RuntimeServiceObservation {
  server: { name: string; version: string };
  protocol: { current: string; minSupported: string };
  health: RuntimeServiceHealth;
  checks: Record<string, RuntimeServiceHealth>;
}

/** Consumer-owned gateway; the adapter removes HTTP paths and response DTOs. */
export interface RuntimeServiceInspector {
  inspect(signal: AbortSignal): Promise<RuntimeServiceObservation>;
}

export interface RuntimeServiceSink {
  checking(): void;
  replace(observation: RuntimeServiceObservation): void;
  unavailable(failure: RuntimeServiceFailure): void;
}

export type RuntimeServiceFailure = { reason: "timeout" } | { reason: "failed"; detail: string };

export interface RuntimeServiceController {
  refresh(): Promise<void>;
  dispose(): void;
}

export const RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS = 10_000;

/**
 * Own one lifecycle-safe inspection sequence. Concurrent refreshes coalesce;
 * dispose aborts transport work and makes every late settlement inert.
 */
export function createRuntimeServiceController(
  inspector: RuntimeServiceInspector,
  sink: RuntimeServiceSink,
): RuntimeServiceController {
  let active = true;
  let attempt: { controller: AbortController; promise: Promise<void> } | null = null;

  return {
    refresh() {
      if (!active) return Promise.resolve();
      if (attempt) return attempt.promise;

      const controller = new AbortController();
      const timeoutError = new Error("runtime_service_inspection_timeout");
      let timeout: ReturnType<typeof setTimeout> | undefined;
      const deadline = new Promise<never>((_resolve, reject) => {
        timeout = setTimeout(() => {
          reject(timeoutError);
          controller.abort();
        }, RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS);
      });
      sink.checking();
      let inspection: Promise<RuntimeServiceObservation>;
      try {
        inspection = inspector.inspect(controller.signal);
      } catch (error) {
        inspection = Promise.reject(error);
      }
      const promise = Promise.race([inspection, deadline])
        .then((observation) => {
          if (active && !controller.signal.aborted) sink.replace(observation);
        })
        .catch((error: unknown) => {
          if (!active) return;
          if (error === timeoutError) {
            sink.unavailable({ reason: "timeout" });
            return;
          }
          if (controller.signal.aborted) return;
          sink.unavailable({
            reason: "failed",
            detail: error instanceof Error ? error.message : String(error),
          });
        })
        .finally(() => {
          if (timeout !== undefined) clearTimeout(timeout);
          if (attempt?.controller === controller) attempt = null;
        });
      attempt = { controller, promise };
      return promise;
    },
    dispose() {
      if (!active) return;
      active = false;
      attempt?.controller.abort();
      attempt = null;
    },
  };
}
