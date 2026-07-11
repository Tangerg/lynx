export interface SingletonPort<T> {
  configure(next: T): () => void;
  get(): T;
}

/**
 * Own a process-local application port with replacement-safe disposal.
 *
 * Plugin reload installs a new adapter before an older cleanup can sometimes
 * be observed by callers. The cleanup therefore clears only the exact adapter
 * instance it installed; a stale disposer can never disconnect its successor.
 */
export function createSingletonPort<T>(notConfiguredMessage: string): SingletonPort<T> {
  let current: T | null = null;

  return {
    configure(next) {
      current = next;
      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        if (current === next) current = null;
      };
    },
    get() {
      if (!current) throw new Error(notConfiguredMessage);
      return current;
    },
  };
}
