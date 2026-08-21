import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

export const UNARY_MUTATION_ATTEMPT_TIMEOUT_MS = 30_000;

export class UnaryMutationSettlementClosedError extends Error {
  override readonly name = "UnaryMutationSettlementClosedError";

  constructor() {
    super("Unary mutation settlement owner is closed");
  }
}

interface PendingUnaryMutation<T> {
  mutation: MutationPromise<T>;
}

export interface UnaryMutationSettler {
  /**
   * Settle one product command. `identity` names the command while its outcome
   * remains unknown; a later call with the same identity replays the retained
   * MutationPromise instead of opening a second logical mutation.
   */
  settle<T>(
    identity: string,
    open: (signal: AbortSignal) => MutationPromise<T>,
    timeoutMs?: number,
  ): Promise<T>;
  /** Revoke this adapter generation and release every process-local identity. */
  dispose(): void;
}

interface UnaryMutationAttempt {
  signal: AbortSignal;
  deadlineExpired(): boolean;
  wait<T>(operation: PromiseLike<T>): Promise<T>;
  accept(): void;
  dispose(): void;
}

function createUnaryMutationAttempt(
  timeoutMs: number,
  lifetime: AbortSignal,
): UnaryMutationAttempt {
  const controller = new AbortController();
  let expired = false;
  let deadlineSettled = false;
  let resolveDeadline!: () => void;
  let rejectDeadline!: (reason: unknown) => void;
  const deadline = new Promise<never>((resolve, reject) => {
    // A released deadline can only settle after its operation already won the
    // race (or before no race was installed). Resolving the `never` branch is
    // therefore an ownership signal, never a product result.
    resolveDeadline = () => resolve(undefined as never);
    rejectDeadline = reject;
  });
  let timer: ReturnType<typeof setTimeout> | undefined = setTimeout(() => {
    timer = undefined;
    deadlineSettled = true;
    expired = true;
    const error = new DOMException("Mutation attempt timed out", "TimeoutError");
    controller.abort(error);
    rejectDeadline(error);
  }, timeoutMs);

  function detachLifetime() {
    lifetime.removeEventListener("abort", abortFromLifetime);
  }

  const clearDeadline = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    detachLifetime();
    if (deadlineSettled) return;
    deadlineSettled = true;
    resolveDeadline();
  };
  function abortFromLifetime() {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    detachLifetime();
    const reason = lifetime.reason ?? new UnaryMutationSettlementClosedError();
    if (!controller.signal.aborted) controller.abort(reason);
    if (deadlineSettled) return;
    deadlineSettled = true;
    rejectDeadline(reason);
  }

  if (lifetime.aborted) abortFromLifetime();
  else lifetime.addEventListener("abort", abortFromLifetime, { once: true });

  return {
    signal: controller.signal,
    deadlineExpired: () => expired,
    // Do not depend on a transport honoring AbortSignal to settle. The signal
    // stops cooperative work; the race independently releases the product
    // command latch when a socket or custom transport ignores cancellation.
    wait: (operation) => Promise.race([operation, deadline]),
    accept: clearDeadline,
    dispose: () => {
      clearDeadline();
      if (!controller.signal.aborted) controller.abort();
    },
  };
}

function driveUnaryMutation<T>(
  mutation: MutationPromise<T>,
  first: UnaryMutationAttempt,
  timeoutMs: number,
  lifetime: AbortSignal,
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
      first.dispose();
      if (!timedOut) {
        if (mutationSettlementIsUnknown(error)) markUnknown();
        throw error;
      }
    }

    const retry = createUnaryMutationAttempt(timeoutMs, lifetime);
    const replay = mutation.retry({ signal: retry.signal });
    replaceMutation(replay);
    try {
      const value = await retry.wait(replay);
      retry.accept();
      return value;
    } catch (error) {
      if (retry.deadlineExpired() || mutationSettlementIsUnknown(error)) markUnknown();
      retry.dispose();
      throw error;
    }
  })();
}

/**
 * Own unresolved unary mutation identities for one Runtime adapter. Product
 * layers choose a semantic identity; transport/idempotency handles remain here.
 * A component unmount or bounded settlement failure therefore cannot turn the
 * next explicit retry into a new Runtime command.
 */
export function createUnaryMutationSettler(): UnaryMutationSettler {
  const pending = new Map<string, PendingUnaryMutation<unknown>[]>();
  const replaying = new Map<string, Promise<unknown>>();
  const lifetime = new AbortController();
  let disposed = false;

  const retain = (identity: string, record: PendingUnaryMutation<unknown>) => {
    if (disposed) return;
    const queue = pending.get(identity) ?? [];
    queue.push(record);
    pending.set(identity, queue);
  };

  const take = <T>(identity: string): PendingUnaryMutation<T> | undefined => {
    const queue = pending.get(identity);
    const record = queue?.shift() as PendingUnaryMutation<T> | undefined;
    if (queue?.length === 0) pending.delete(identity);
    return record;
  };

  return {
    settle<T>(
      identity: string,
      open: (signal: AbortSignal) => MutationPromise<T>,
      timeoutMs: number = UNARY_MUTATION_ATTEMPT_TIMEOUT_MS,
    ): Promise<T> {
      if (disposed) return Promise.reject(new UnaryMutationSettlementClosedError());
      const activeReplay = replaying.get(identity) as Promise<T> | undefined;
      if (activeReplay) return activeReplay;

      // Only a command that already returned settlement-unknown may be reused.
      // Two fresh same-shaped calls can still be separate product intents; their
      // application owner, not the transport adapter, decides whether to join.
      const retained = take<T>(identity);

      const first = createUnaryMutationAttempt(timeoutMs, lifetime.signal);
      let mutation: MutationPromise<T>;
      try {
        mutation = retained
          ? retained.mutation.retry({ signal: first.signal })
          : open(first.signal);
      } catch (error) {
        first.dispose();
        if (retained) retain(identity, retained as PendingUnaryMutation<unknown>);
        return Promise.reject(error);
      }

      const record = retained ?? { mutation };
      record.mutation = mutation;

      let unknown = false;
      const settlement = driveUnaryMutation(
        mutation,
        first,
        timeoutMs,
        lifetime.signal,
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
            if (unknown) retain(identity, record as PendingUnaryMutation<unknown>);
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
      lifetime.abort(new UnaryMutationSettlementClosedError());
      pending.clear();
      replaying.clear();
    },
  };
}
