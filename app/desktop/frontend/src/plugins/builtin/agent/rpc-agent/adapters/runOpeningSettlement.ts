import type { MutationPromise } from "@/rpc";

export const RUN_OPENING_ATTEMPT_TIMEOUT_MS = 30_000;

interface OpeningAttempt {
  readonly signal: AbortSignal;
  readonly deadlineExpired: () => boolean;
  accept(): void;
  dispose(): void;
}

function createOpeningAttempt(parent: AbortSignal | undefined, timeoutMs: number): OpeningAttempt {
  const controller = new AbortController();
  let expired = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const clearDeadline = () => {
    if (timer === undefined) return;
    clearTimeout(timer);
    timer = undefined;
  };
  const abortFromParent = () => {
    clearDeadline();
    controller.abort(parent?.reason);
  };

  timer = setTimeout(() => {
    timer = undefined;
    expired = true;
    controller.abort(new DOMException("Run opening attempt timed out", "TimeoutError"));
  }, timeoutMs);

  if (parent?.aborted) abortFromParent();
  else parent?.addEventListener("abort", abortFromParent, { once: true });

  return {
    signal: controller.signal,
    deadlineExpired: () => expired,
    // The winning attempt's signal continues to own the returned event stream.
    // Only its opening deadline is released; the parent session signal remains
    // linked until the stream is stopped, replaced, or the driver unmounts.
    accept: clearDeadline,
    dispose: () => {
      clearDeadline();
      parent?.removeEventListener("abort", abortFromParent);
      if (!controller.signal.aborted) controller.abort();
    },
  };
}

/**
 * Settle one replayable streaming opening without conflating its handshake
 * deadline with the accepted stream's lifetime.
 *
 * Runtime owns same-key replay; this Adapter owns the product deadline. A
 * timed-out first delivery is ambiguous, so the second delivery reuses the
 * original MutationPromise identity with a fresh signal. The budget is finite:
 * a second timeout returns to the opening controller and releases its latch.
 */
export async function settleRunOpening<T>(
  open: (signal: AbortSignal) => MutationPromise<T>,
  parent?: AbortSignal,
  timeoutMs: number = RUN_OPENING_ATTEMPT_TIMEOUT_MS,
): Promise<T> {
  const first = createOpeningAttempt(parent, timeoutMs);
  let mutation: MutationPromise<T>;
  try {
    mutation = open(first.signal);
  } catch (error) {
    first.dispose();
    throw error;
  }

  try {
    const value = await mutation;
    first.accept();
    return value;
  } catch (error) {
    const timedOut = first.deadlineExpired();
    first.dispose();
    if (!timedOut || parent?.aborted) throw error;
  }

  const retry = createOpeningAttempt(parent, timeoutMs);
  try {
    const value = await mutation.retry({ signal: retry.signal });
    retry.accept();
    return value;
  } catch (error) {
    retry.dispose();
    throw error;
  }
}
