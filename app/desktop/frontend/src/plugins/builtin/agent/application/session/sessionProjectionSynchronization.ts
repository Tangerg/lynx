export interface SessionProjectionSynchronization {
  /** Coalesce an authoritative refresh request. A request made while the live
   *  stream owns the session is retained until that stream becomes idle. */
  request(): void;
  /** Notify the coordinator after the live stream has folded its queued tail. */
  liveStreamSettled(): void;
  dispose(): void;
}

interface SessionProjectionSynchronizationOptions {
  isLiveStreamActive: () => boolean;
  synchronize: () => Promise<void>;
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

  const drain = (): void => {
    if (disposed || synchronizing || !requested || isLiveStreamActive()) return;
    requested = false;
    synchronizing = true;
    void synchronize().finally(() => {
      synchronizing = false;
      drain();
    });
  };

  return {
    request() {
      if (disposed) return;
      requested = true;
      drain();
    },
    liveStreamSettled: drain,
    dispose() {
      disposed = true;
      requested = false;
    },
  };
}
