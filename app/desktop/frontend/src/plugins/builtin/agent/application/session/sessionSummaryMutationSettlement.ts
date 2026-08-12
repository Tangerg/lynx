interface SessionSummaryMutationResult {
  revision: number;
}

interface SessionSummaryMutationQueue {
  tail: Promise<void>;
  revision: number | null;
}

const queues = new Map<string, SessionSummaryMutationQueue>();

/**
 * Serialize local conditional Session summary writes and carry each committed
 * revision into the next local intent. Runtime still owns CAS and concurrent
 * remote writers; this owner only prevents our own rename/favorite commands
 * from racing with the same stale revision.
 */
export function settleSessionSummaryMutation<T extends SessionSummaryMutationResult>(
  sessionId: string,
  expectedRevision: number,
  execute: (revision: number) => Promise<T>,
): Promise<T> {
  const queue = queues.get(sessionId) ?? {
    tail: Promise.resolve(),
    revision: null,
  };
  queues.set(sessionId, queue);

  const result = queue.tail.then(() =>
    execute(Math.max(expectedRevision, queue.revision ?? expectedRevision)),
  );
  const settled = result.then(
    (value) => {
      queue.revision = value.revision;
    },
    () => undefined,
  );
  queue.tail = settled;
  void settled.finally(() => {
    if (queues.get(sessionId)?.tail === settled) queues.delete(sessionId);
  });
  return result;
}
