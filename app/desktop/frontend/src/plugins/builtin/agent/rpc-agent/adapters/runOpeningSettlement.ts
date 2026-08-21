import { mutationSettlementIsUnknown, type MutationPromise } from "@/rpc";

export const RUN_OPENING_ATTEMPT_TIMEOUT_MS = 30_000;

export class RunOpeningSettlementClosedError extends Error {
  override readonly name = "RunOpeningSettlementClosedError";

  constructor() {
    super("Run opening settlement owner is closed");
  }
}

interface OpeningAttempt {
  readonly signal: AbortSignal;
  readonly deadlineExpired: () => boolean;
  wait<T>(operation: PromiseLike<T>): Promise<T>;
  accept(): void;
  dispose(): void;
}

interface PendingRunOpening<T> {
  mutation: MutationPromise<T>;
}

export interface RunOpeningSettler {
  settle<T>(
    identity: string,
    open: (signal: AbortSignal) => MutationPromise<T>,
    parent?: AbortSignal,
    timeoutMs?: number,
  ): Promise<T>;
  /** Revoke this adapter generation, including any accepted event stream. */
  dispose(): void;
}

function abortReason(signal: AbortSignal): unknown {
  return signal.reason ?? new DOMException("Run opening canceled", "AbortError");
}

function signalIsAborted(signal: AbortSignal | undefined): boolean {
  return signal?.aborted === true;
}

function createOpeningAttempt(parent: AbortSignal | undefined, timeoutMs: number): OpeningAttempt {
  const controller = new AbortController();
  let expired = false;
  let deadlineSettled = false;
  let resolveDeadline!: () => void;
  let rejectDeadline!: (reason: unknown) => void;
  const deadline = new Promise<never>((resolve, reject) => {
    resolveDeadline = () => resolve(undefined as never);
    rejectDeadline = reject;
  });
  let timer: ReturnType<typeof setTimeout> | undefined;

  const clearDeadline = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  };
  const releaseDeadline = () => {
    clearDeadline();
    if (deadlineSettled) return;
    deadlineSettled = true;
    resolveDeadline();
  };
  const abortFromParent = () => {
    clearDeadline();
    controller.abort(parent?.reason);
    if (deadlineSettled) return;
    deadlineSettled = true;
    rejectDeadline(abortReason(parent!));
  };

  timer = setTimeout(() => {
    timer = undefined;
    deadlineSettled = true;
    expired = true;
    const error = new DOMException("Run opening attempt timed out", "TimeoutError");
    controller.abort(error);
    rejectDeadline(error);
  }, timeoutMs);

  if (parent?.aborted) abortFromParent();
  else parent?.addEventListener("abort", abortFromParent, { once: true });

  return {
    signal: controller.signal,
    deadlineExpired: () => expired,
    // A transport is expected to honor the attempt signal, but the product
    // deadline must still settle if a socket/custom transport ignores it.
    wait: (operation) => Promise.race([operation, deadline]),
    // The winning attempt's signal continues to own the returned event stream.
    // Only its opening deadline is released; the parent session signal remains
    // linked until the stream is stopped, replaced, or the driver unmounts.
    accept: releaseDeadline,
    dispose: () => {
      releaseDeadline();
      parent?.removeEventListener("abort", abortFromParent);
      if (!controller.signal.aborted) controller.abort();
    },
  };
}

function driveRunOpening<T>(
  mutation: MutationPromise<T>,
  first: OpeningAttempt,
  parent: AbortSignal | undefined,
  timeoutMs: number,
  markUnknown: () => void,
  replaceMutation: (mutation: MutationPromise<T>) => void,
): Promise<T> {
  return (async () => {
    try {
      const value = await first.wait(mutation);
      first.accept();
      return value;
    } catch (error) {
      const timedOut = first.deadlineExpired();
      const canceled = signalIsAborted(parent);
      first.dispose();
      if (!timedOut || canceled) {
        if (canceled || mutationSettlementIsUnknown(error)) markUnknown();
        throw error;
      }
    }

    const retry = createOpeningAttempt(parent, timeoutMs);
    let replay: MutationPromise<T>;
    try {
      replay = mutation.retry({ signal: retry.signal });
    } catch (error) {
      retry.dispose();
      throw error;
    }
    replaceMutation(replay);
    try {
      const value = await retry.wait(replay);
      retry.accept();
      return value;
    } catch (error) {
      if (
        retry.deadlineExpired() ||
        signalIsAborted(parent) ||
        mutationSettlementIsUnknown(error)
      ) {
        markUnknown();
      }
      retry.dispose();
      throw error;
    }
  })();
}

/** Own same-key streaming openings until their channel-a result is known. */
export function createRunOpeningSettler(): RunOpeningSettler {
  const pending = new Map<string, PendingRunOpening<unknown>[]>();
  const replaying = new Map<string, Promise<unknown>>();
  const lifetime = new AbortController();
  let disposed = false;

  const retain = (identity: string, record: PendingRunOpening<unknown>) => {
    if (disposed) return;
    const queue = pending.get(identity) ?? [];
    queue.push(record);
    pending.set(identity, queue);
  };

  const take = <T>(identity: string): PendingRunOpening<T> | undefined => {
    const queue = pending.get(identity);
    const record = queue?.shift() as PendingRunOpening<T> | undefined;
    if (queue?.length === 0) pending.delete(identity);
    return record;
  };

  return {
    settle<T>(
      identity: string,
      open: (signal: AbortSignal) => MutationPromise<T>,
      parent?: AbortSignal,
      timeoutMs: number = RUN_OPENING_ATTEMPT_TIMEOUT_MS,
    ): Promise<T> {
      if (disposed) return Promise.reject(new RunOpeningSettlementClosedError());
      const activeReplay = replaying.get(identity) as Promise<T> | undefined;
      if (activeReplay) return activeReplay;
      const retained = take<T>(identity);
      const ownership = parent ? AbortSignal.any([parent, lifetime.signal]) : lifetime.signal;

      const first = createOpeningAttempt(ownership, timeoutMs);
      let mutation: MutationPromise<T>;
      try {
        mutation = retained
          ? retained.mutation.retry({ signal: first.signal })
          : open(first.signal);
      } catch (error) {
        first.dispose();
        if (retained) retain(identity, retained as PendingRunOpening<unknown>);
        return Promise.reject(error);
      }

      const record = retained ?? { mutation };
      record.mutation = mutation;

      let unknown = false;
      const settlement = driveRunOpening(
        mutation,
        first,
        ownership,
        timeoutMs,
        () => {
          unknown = true;
        },
        (replay) => {
          record.mutation = replay;
        },
      );
      const tracked = settlement
        .then(
          (value) => value,
          (error: unknown) => {
            if (unknown) retain(identity, record as PendingRunOpening<unknown>);
            throw error;
          },
        )
        .finally(() => {
          if (replaying.get(identity) === tracked) replaying.delete(identity);
        });
      if (retained) replaying.set(identity, tracked as Promise<unknown>);
      return tracked;
    },
    dispose(): void {
      if (disposed) return;
      disposed = true;
      lifetime.abort(new RunOpeningSettlementClosedError());
      pending.clear();
      replaying.clear();
    },
  };
}
