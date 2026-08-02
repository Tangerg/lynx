export interface MutationPromise<T> extends Promise<T> {
  /** Stable identity of this invocation. Persist it before awaiting when a retry
   * must survive a client restart. */
  readonly idempotencyKey: string;
  /** Execute the same invocation again with the same idempotency key. */
  retry(): MutationPromise<T>;
}

export function createMutationPromise<T>(
  execute: (idempotencyKey: string) => Promise<T>,
  idempotencyKey: string = crypto.randomUUID(),
): MutationPromise<T> {
  const promise = Promise.resolve().then(() => execute(idempotencyKey));
  return Object.defineProperties(promise, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: {
      enumerable: true,
      value: () => createMutationPromise(execute, idempotencyKey),
    },
  }) as MutationPromise<T>;
}
