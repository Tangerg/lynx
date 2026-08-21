export type RuntimeServiceHealth = "ready" | "degraded" | "unhealthy";

export interface RuntimeServiceObservation {
  server: { name: string; version: string };
  protocolVersion: string;
  health: RuntimeServiceHealth;
  checks: Record<string, RuntimeServiceHealth>;
}

/** Opaque identity reported by one Runtime process incarnation. */
export type RuntimeProcessGeneration = string;

export interface RuntimeConnectionInspection<Capabilities> {
  /** The exact process generation observed by every member of this inspection. */
  processGeneration: RuntimeProcessGeneration;
  service: RuntimeServiceObservation;
  capabilities: Capabilities;
}

/** Consumer-owned gateway; the adapter removes HTTP paths and response DTOs. */
export interface RuntimeConnectionInspector<Capabilities> {
  inspect(signal: AbortSignal): Promise<RuntimeConnectionInspection<Capabilities>>;
}

export interface RuntimeServiceSink<Capabilities> {
  checking(): void;
  replace(inspection: RuntimeConnectionInspection<Capabilities>): void;
  unavailable(failure: RuntimeServiceFailure): void;
}

export type RuntimeServiceFailure = { reason: "timeout" } | { reason: "failed"; detail: string };

export interface RuntimeServiceController {
  start(): void;
  refresh(): Promise<void>;
  /** Supersede any inspection admitted before a proven transport loss and inspect
   *  again without waiting for that predecessor to cooperate. */
  recover(): Promise<void>;
  dispose(): void;
}

export const RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS = 10_000;
export const RUNTIME_SERVICE_HEALTHY_POLL_MS = 30_000;
export const RUNTIME_SERVICE_RETRY_BASE_MS = 1_000;
export const RUNTIME_SERVICE_RETRY_CAP_MS = 30_000;

/**
 * Own one lifecycle-safe inspection sequence. Concurrent refreshes coalesce;
 * dispose aborts transport work and makes every late settlement inert.
 */
export function createRuntimeServiceController<Capabilities>(
  inspector: RuntimeConnectionInspector<Capabilities>,
  sink: RuntimeServiceSink<Capabilities>,
): RuntimeServiceController {
  let active = true;
  let monitoring = false;
  let failures = 0;
  let scheduled: ReturnType<typeof setTimeout> | undefined;
  let attempt: {
    controller: AbortController;
    promise: Promise<void>;
    releaseDeadline: () => void;
  } | null = null;

  const clearSchedule = () => {
    if (scheduled !== undefined) clearTimeout(scheduled);
    scheduled = undefined;
  };

  const scheduleNext = (delay: number) => {
    if (!active || !monitoring) return;
    clearSchedule();
    scheduled = setTimeout(() => {
      scheduled = undefined;
      void inspect(false);
    }, delay);
  };

  const inspect = (announce: boolean): Promise<void> => {
    if (!active) return Promise.resolve();
    if (attempt) return attempt.promise;
    clearSchedule();

    const controller = new AbortController();
    const timeoutError = new Error("runtime_service_inspection_timeout");
    let deadlineSettled = false;
    let resolveDeadline!: () => void;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    const deadline = new Promise<never>((resolve, reject) => {
      resolveDeadline = () => resolve(undefined as never);
      timeout = setTimeout(() => {
        timeout = undefined;
        deadlineSettled = true;
        reject(timeoutError);
        controller.abort();
      }, RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS);
    });
    const releaseDeadline = () => {
      if (timeout !== undefined) clearTimeout(timeout);
      timeout = undefined;
      if (deadlineSettled) return;
      deadlineSettled = true;
      resolveDeadline();
    };
    if (announce) sink.checking();
    let inspection: Promise<RuntimeConnectionInspection<Capabilities>>;
    try {
      inspection = inspector.inspect(controller.signal);
    } catch (error) {
      inspection = Promise.reject(error);
    }
    let succeeded = false;
    const promise = Promise.race([inspection, deadline])
      .then((result) => {
        if (!active || controller.signal.aborted) return;
        succeeded = true;
        failures = 0;
        sink.replace(result);
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error === timeoutError) {
          failures += 1;
          sink.unavailable({ reason: "timeout" });
          return;
        }
        if (controller.signal.aborted) return;
        failures += 1;
        sink.unavailable({
          reason: "failed",
          detail: error instanceof Error ? error.message : String(error),
        });
      })
      .finally(() => {
        releaseDeadline();
        const ownsAttempt = attempt?.controller === controller;
        if (ownsAttempt) attempt = null;
        // A forced recovery publishes its successor attempt before this retired
        // one settles. Its cleanup may neither clear the successor nor install a
        // competing retry/poll timer.
        if (!active || !monitoring || !ownsAttempt) return;
        if (succeeded) {
          scheduleNext(RUNTIME_SERVICE_HEALTHY_POLL_MS);
          return;
        }
        const exponent = Math.max(0, failures - 1);
        scheduleNext(
          Math.min(RUNTIME_SERVICE_RETRY_BASE_MS * 2 ** exponent, RUNTIME_SERVICE_RETRY_CAP_MS),
        );
      });
    attempt = { controller, promise, releaseDeadline };
    return promise;
  };

  return {
    start() {
      if (!active || monitoring) return;
      monitoring = true;
      void inspect(true);
    },
    refresh() {
      return inspect(true);
    },
    recover() {
      if (!active) return Promise.resolve();
      clearSchedule();
      const predecessor = attempt;
      if (predecessor) {
        // Abort is the cooperative path. Releasing the inspection deadline is
        // the non-cooperative path that lets the successor start in this turn;
        // the foreign promise remains observed by Promise.race.
        attempt = null;
        predecessor.controller.abort();
        predecessor.releaseDeadline();
      }
      return inspect(false);
    },
    dispose() {
      if (!active) return;
      active = false;
      monitoring = false;
      clearSchedule();
      attempt?.releaseDeadline();
      attempt?.controller.abort();
      attempt = null;
    },
  };
}
