import { createPublicationSlot } from "../publicationSlot";

export interface SingletonPort<T> {
  configure(next: T): () => void;
  get(): T;
  /**
   * The adapter if one is installed, else null — for the callers that have a
   * correct answer without it. `get()` throws because most callers do not: reading
   * a port before its adapter exists is a wiring bug there. But a question like
   * "what has the server negotiated" is answerable before install — nothing has —
   * and making that caller catch a thrown wiring error would hide real ones.
   */
  peek(): T | null;
}

/**
 * Own a process-local application port with replacement-safe disposal.
 *
 * Plugin reload installs a new adapter before an older cleanup can sometimes
 * be observed by callers. The cleanup therefore clears only the exact adapter
 * instance it installed; a stale disposer can never disconnect its successor.
 */
export function createSingletonPort<T>(notConfiguredMessage: string): SingletonPort<T> {
  const slot = createPublicationSlot<{ value: T }>();

  return {
    configure(next) {
      const published = { value: next };
      slot.publish(published, () => undefined);
      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        slot.withdraw(published);
      };
    },
    get() {
      const current = slot.current();
      if (!current) throw new Error(notConfiguredMessage);
      return current.value;
    },
    peek() {
      return slot.current()?.value ?? null;
    },
  };
}
