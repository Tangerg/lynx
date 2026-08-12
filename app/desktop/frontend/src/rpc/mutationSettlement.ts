import type { MutationPromise } from "./mutation";

export const UNARY_MUTATION_ATTEMPT_TIMEOUT_MS = 30_000;

interface UnaryMutationAttempt {
  signal: AbortSignal;
  deadlineExpired(): boolean;
  wait<T>(operation: PromiseLike<T>): Promise<T>;
  accept(): void;
  dispose(): void;
}

function createUnaryMutationAttempt(timeoutMs: number): UnaryMutationAttempt {
  const controller = new AbortController();
  let expired = false;
  let rejectDeadline!: (reason: unknown) => void;
  const deadline = new Promise<never>((_resolve, reject) => {
    rejectDeadline = reject;
  });
  let timer: ReturnType<typeof setTimeout> | undefined = setTimeout(() => {
    timer = undefined;
    expired = true;
    const error = new DOMException("Mutation attempt timed out", "TimeoutError");
    controller.abort(error);
    rejectDeadline(error);
  }, timeoutMs);

  const clearDeadline = () => {
    if (timer === undefined) return;
    clearTimeout(timer);
    timer = undefined;
  };

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

/**
 * Give one replayable unary command a finite product deadline. Runtime owns
 * same-key idempotency; this transport helper owns two bounded delivery
 * attempts and never manufactures a second logical command.
 */
export async function settleUnaryMutation<T>(
  open: (signal: AbortSignal) => MutationPromise<T>,
  timeoutMs: number = UNARY_MUTATION_ATTEMPT_TIMEOUT_MS,
): Promise<T> {
  const first = createUnaryMutationAttempt(timeoutMs);
  let mutation: MutationPromise<T>;
  try {
    mutation = open(first.signal);
  } catch (error) {
    first.dispose();
    throw error;
  }

  try {
    const value = await first.wait(mutation);
    first.accept();
    return value;
  } catch (error) {
    const timedOut = first.deadlineExpired();
    first.dispose();
    if (!timedOut) throw error;
  }

  const retry = createUnaryMutationAttempt(timeoutMs);
  try {
    const value = await retry.wait(mutation.retry({ signal: retry.signal }));
    retry.accept();
    return value;
  } catch (error) {
    retry.dispose();
    throw error;
  }
}
