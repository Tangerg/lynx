export interface SessionProjectionSynchronization {
  /** Coalesce an authoritative refresh request. A request made while the live
   *  stream owns the session is retained until that stream becomes idle. The
   *  returned promise settles with the refresh's authoritative commit fact. */
  request(): Promise<boolean>;
  /** Notify the coordinator after the live stream has folded its queued tail. */
  liveStreamSettled(): void;
  dispose(): void;
}

interface SessionProjectionSynchronizationOptions {
  isLiveStreamActive: () => boolean;
  synchronize: () => Promise<boolean>;
}

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
  let pendingWaiters: Array<(committed: boolean) => void> = [];
  let activeWaiters: Array<(committed: boolean) => void> = [];

  const drain = (): void => {
    if (disposed || synchronizing || !requested || isLiveStreamActive()) return;
    requested = false;
    synchronizing = true;
    const waiters = pendingWaiters;
    pendingWaiters = [];
    activeWaiters = waiters;
    void synchronize()
      .catch(() => false)
      .then((committed) => {
        for (const settle of waiters) settle(committed);
      })
      .finally(() => {
        if (activeWaiters === waiters) activeWaiters = [];
        synchronizing = false;
        drain();
      });
  };

  return {
    request() {
      if (disposed) return Promise.resolve(false);
      return new Promise<boolean>((resolve) => {
        pendingWaiters.push(resolve);
        requested = true;
        drain();
      });
    },
    liveStreamSettled: drain,
    dispose() {
      disposed = true;
      requested = false;
      const waiters = pendingWaiters;
      pendingWaiters = [];
      for (const settle of waiters) settle(false);
      const inFlight = activeWaiters;
      activeWaiters = [];
      for (const settle of inFlight) settle(false);
    },
  };
}
