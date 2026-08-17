export interface SessionProjectionSynchronization {
  /** Coalesce an authoritative refresh request. A request made while the live
   *  stream owns the session is retained until that stream becomes idle. The
   *  returned promise settles with the refresh's authoritative commit fact. */
  request(): Promise<boolean>;
  /** Supersede the active Runtime generation. The current synchronization is
   * retired even when one of its reads does not cooperate with cancellation. */
  replace(): Promise<boolean>;
  /** Revoke the active and queued generation without admitting a successor. */
  retire(): void;
  /** Notify the coordinator after the live stream has folded its queued tail. */
  liveStreamSettled(): void;
  dispose(): void;
}

interface SessionProjectionSynchronizationOptions {
  isLiveStreamActive: () => boolean;
  synchronize: (signal: AbortSignal) => Promise<boolean>;
}

const ABORTED = Symbol("session-projection-synchronization.aborted");

/**
 * Serializes the two fact channels which feed one mounted session projection.
 *
 * Run events are the ordered, low-latency owner while a stream is active. The
 * durable snapshot is the reconciliation owner while no stream is active.
 * Change notifications may arrive at any time, so requests are coalesced and
 * drained only at that ownership boundary instead of racing both writers.
 */
export function createSessionProjectionSynchronization({
  isLiveStreamActive,
  synchronize,
}: SessionProjectionSynchronizationOptions): SessionProjectionSynchronization {
  let requested = false;
  let synchronizing = false;
  let disposed = false;
  let activeAbort: AbortController | null = null;
  let pendingWaiters: Array<(committed: boolean) => void> = [];
  let activeWaiters: Array<(committed: boolean) => void> = [];

  const drain = (): void => {
    if (disposed || synchronizing || !requested || isLiveStreamActive()) return;
    requested = false;
    synchronizing = true;
    const waiters = pendingWaiters;
    pendingWaiters = [];
    activeWaiters = waiters;
    const controller = new AbortController();
    activeAbort = controller;
    void settleBeforeAbort(synchronize(controller.signal), controller.signal)
      .then((committed) => (committed === ABORTED ? false : committed))
      .catch(() => false)
      .then((committed) => {
        for (const settle of waiters) settle(committed);
      })
      .finally(() => {
        if (activeAbort === controller) activeAbort = null;
        if (activeWaiters === waiters) activeWaiters = [];
        synchronizing = false;
        drain();
      });
  };

  const enqueue = (): Promise<boolean> => {
    if (disposed) return Promise.resolve(false);
    return new Promise<boolean>((resolve) => {
      pendingWaiters.push(resolve);
      requested = true;
      drain();
    });
  };

  const retire = (): void => {
    requested = false;
    activeAbort?.abort();
    const pending = pendingWaiters;
    pendingWaiters = [];
    for (const settle of pending) settle(false);
    const inFlight = activeWaiters;
    activeWaiters = [];
    for (const settle of inFlight) settle(false);
  };

  return {
    request: enqueue,
    replace() {
      activeAbort?.abort();
      return enqueue();
    },
    retire,
    liveStreamSettled: drain,
    dispose() {
      disposed = true;
      retire();
      activeAbort = null;
    },
  };
}

/** Observe a dependency that may ignore cancellation without allowing it to
 * retain synchronization ownership. Its late settlement remains handled. */
function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
): Promise<T | typeof ABORTED> {
  if (signal.aborted) {
    void operation.catch(() => undefined);
    return Promise.resolve(ABORTED);
  }
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ABORTED);
    };
    signal.addEventListener("abort", onAbort, { once: true });
    operation.then(
      (value) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}
