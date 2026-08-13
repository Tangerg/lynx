import { isErrorType, RpcProtocolError, RpcTransportError } from "./errors";

export interface MutationAttemptOptions {
  /** Cancellation belongs to one delivery attempt, not the logical mutation.
   * A caller may therefore retry the same idempotency key with a fresh signal. */
  signal?: AbortSignal;
}

export interface MutationPromise<T> extends Promise<T> {
  /** Stable identity of this invocation. Persist it before awaiting when a retry
   * must survive a client restart. */
  readonly idempotencyKey: string;
  /** Execute the same invocation again with the same idempotency key. */
  retry(options?: MutationAttemptOptions): MutationPromise<T>;
}

type MutationExecution<T> = (idempotencyKey: string, options: MutationAttemptOptions) => Promise<T>;

function retryableTransportFailure(error: unknown): error is RpcTransportError {
  if (!(error instanceof RpcTransportError)) return false;
  // A JSON-RPC response never arrived. Network/body failures and server-side
  // transport failures have ambiguous settlement; ordinary 4xx responses are
  // definitive admission refusals and should return immediately.
  return error.status === undefined || error.status === 408 || (error.status ?? 0) >= 500;
}

/** Whether a failed attempt still leaves business commit unknown to the client.
 * Product settlement owners retain the MutationPromise only for these failures;
 * a typed business refusal is a complete response and may release the identity. */
export function mutationSettlementIsUnknown(error: unknown): boolean {
  if (error instanceof RpcProtocolError) return true;
  if (retryableTransportFailure(error)) return true;
  return isErrorType(error, "idempotency_in_progress");
}

function abortReason(signal: AbortSignal): unknown {
  return signal.reason ?? new DOMException("The operation was aborted", "AbortError");
}

async function waitForReplay(seconds: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) throw abortReason(signal);
  await new Promise<void>((resolve, reject) => {
    const finish = () => {
      signal?.removeEventListener("abort", abort);
      resolve();
    };
    const abort = () => {
      clearTimeout(timer);
      reject(abortReason(signal!));
    };
    const timer = setTimeout(finish, seconds * 1_000);
    signal?.addEventListener("abort", abort, { once: true });
  });
}

/**
 * Drive one logical mutation to a determinate response.
 *
 * Commands are registered with Runtime replay semantics: the same key never
 * executes the business handler twice. One transport recovery replay closes
 * the common "commit succeeded, response was lost" window. If that replay
 * meets the original execution, Runtime supplies the one typed backoff we may
 * honor. Budgets are deliberately finite so a dead transport/claim still
 * returns control to the product, whose explicit retry keeps the same key.
 */
async function settleMutation<T>(
  execute: MutationExecution<T>,
  idempotencyKey: string,
  options: MutationAttemptOptions,
): Promise<T> {
  let replayedTransportFailure = false;
  let waitedForInProgress = false;
  for (;;) {
    try {
      return await execute(idempotencyKey, options);
    } catch (error) {
      if (options.signal?.aborted) throw error;
      if (!replayedTransportFailure && retryableTransportFailure(error)) {
        replayedTransportFailure = true;
        continue;
      }
      if (!waitedForInProgress && isErrorType(error, "idempotency_in_progress")) {
        waitedForInProgress = true;
        await waitForReplay(error.data.retryAfterSeconds, options.signal);
        continue;
      }
      throw error;
    }
  }
}

export function createMutationPromise<T>(
  execute: MutationExecution<T>,
  idempotencyKey: string = crypto.randomUUID(),
  options: MutationAttemptOptions = {},
): MutationPromise<T> {
  const promise = Promise.resolve().then(() => settleMutation(execute, idempotencyKey, options));
  return Object.defineProperties(promise, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: {
      enumerable: true,
      value: (retryOptions: MutationAttemptOptions = options) =>
        createMutationPromise(execute, idempotencyKey, retryOptions),
    },
  }) as MutationPromise<T>;
}
