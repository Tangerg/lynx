export interface SerialTaskQueue {
  run<T>(task: () => Promise<T>): Promise<T>;
}

export interface KeyedSerialTaskQueue<K> {
  run<T>(key: K, task: () => Promise<T>): Promise<T>;
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
