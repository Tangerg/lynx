// Push-pull async channel — single-consumer AsyncIterableIterator backed
// by an internal buffer + FIFO waiter queue. The four sites that used
// to roll their own version (memory transport, http transport, run
// event stream, terminal/background stream) all delegate to this now.
//
// Contract:
//   - push(value) buffers OR resolves a waiting next() call.
//   - close() drains pending values to the consumer and then returns
//     done=true. Idempotent.
//   - iterator() returns a fresh iterator object; the underlying queue
//     is shared, so callers must treat the channel as single-consumer
//     (multiple iterators would race for buffered values).
//   - iterator().return() closes the channel — used by `for await` to
//     clean up when the loop breaks out early.

export interface PushPullChannel<T> {
  push(value: T): void;
  /** Close the channel. Idempotent. */
  close(): void;
  readonly closed: boolean;
  iterator(): AsyncIterableIterator<T>;
}

export function createPushPullChannel<T>(): PushPullChannel<T> {
  const buffer: T[] = [];
  const waiters: Array<(result: IteratorResult<T>) => void> = [];
  let isClosed = false;

  function push(value: T): void {
    if (isClosed) return;
    const w = waiters.shift();
    if (w) {
      w({ value, done: false });
    } else {
      buffer.push(value);
    }
  }

  function close(): void {
    if (isClosed) return;
    isClosed = true;
    for (const w of waiters.splice(0)) w({ value: undefined as never, done: true });
  }

  return {
    push,
    close,
    get closed() {
      return isClosed;
    },
    iterator(): AsyncIterableIterator<T> {
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        async next(): Promise<IteratorResult<T>> {
          if (buffer.length > 0) return { value: buffer.shift()!, done: false };
          if (isClosed) return { value: undefined as never, done: true };
          return new Promise<IteratorResult<T>>((resolve) => {
            waiters.push(resolve);
          });
        },
        async return(): Promise<IteratorResult<T>> {
          close();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
