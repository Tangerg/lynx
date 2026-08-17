export interface SerialTaskQueue {
  run<T>(task: () => Promise<T>): Promise<T>;
}

export interface KeyedSerialTaskQueue<K> {
  run<T>(key: K, task: () => Promise<T>): Promise<T>;
}

/**
 * Owns every asynchronous settlement admitted by one replaceable task
 * generation. Completed work unregisters immediately; retirement rejects only
 * genuinely pending work, even when the underlying dependency ignores
 * cancellation.
 */
export class RetirableTaskCohort {
  readonly #retiredError: Error;
  readonly #settlers = new Set<() => void>();
  #retired = false;

  constructor(retiredError: Error) {
    this.#retiredError = retiredError;
  }

  get retired(): boolean {
    return this.#retired;
  }

  assertCurrent(): void {
    if (this.#retired) throw this.#retiredError;
  }

  settle<T>(operation: PromiseLike<T>): Promise<T> {
    this.assertCurrent();
    return new Promise<T>((resolve, reject) => {
      let pending = true;
      const finish = () => {
        if (!pending) return false;
        pending = false;
        this.#settlers.delete(retire);
        return true;
      };
      const retire = () => {
        if (finish()) reject(this.#retiredError);
      };
      this.#settlers.add(retire);
      operation.then(
        (value) => {
          if (finish()) resolve(value);
        },
        (error: unknown) => {
          if (finish()) reject(error);
        },
      );
      if (this.#retired) retire();
    });
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
  }
}

export function createSerialTaskQueue(): SerialTaskQueue {
  let tail = Promise.resolve();
  return {
    run<T>(task: () => Promise<T>): Promise<T> {
      const result = tail.then(task);
      tail = result.then(
        () => undefined,
        () => undefined,
      );
      return result;
    },
  };
}

export function createKeyedSerialTaskQueue<K>(): KeyedSerialTaskQueue<K> {
  const tails = new Map<K, Promise<void>>();
  return {
    run<T>(key: K, task: () => Promise<T>): Promise<T> {
      const result = (tails.get(key) ?? Promise.resolve()).then(task);
      const settled = result.then(
        () => undefined,
        () => undefined,
      );
      tails.set(key, settled);
      void settled.finally(() => {
        if (tails.get(key) === settled) tails.delete(key);
      });
      return result;
    },
  };
}
