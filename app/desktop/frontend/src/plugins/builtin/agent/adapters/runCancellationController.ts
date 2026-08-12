interface RunCancellationTarget {
  terminal: boolean;
  viewRevision: number;
}

interface RunCancellationControllerOptions<Response> {
  isCancelled: () => boolean;
  markInteracted: () => void;
  readTarget: (runId: string) => RunCancellationTarget | null;
  execute: (runId: string) => Promise<Response>;
  commitIfCurrent: (response: Response, expectedViewRevision: number) => boolean;
  revalidateTerminal: (runId: string) => Promise<boolean>;
  onSettled: () => void;
  onFailure: (runId: string, error: unknown) => void;
}

export interface RunCancellationController {
  cancel(runId: string): void;
}

/** Own one cancellation command per Run until its response is settled.
 *
 * A successful response is a snapshot captured at command commit time, so it
 * may only fold while the material view still has the revision from which the
 * command started. A failed command is revalidated through the neutral Agent
 * projection: another client reaching terminal is objective success, while an
 * active authoritative Run preserves the original command failure.
 */
export function createRunCancellationController<Response>({
  isCancelled,
  markInteracted,
  readTarget,
  execute,
  commitIfCurrent,
  revalidateTerminal,
  onSettled,
  onFailure,
}: RunCancellationControllerOptions<Response>): RunCancellationController {
  const pending = new Set<string>();

  return {
    cancel(runId) {
      const target = readTarget(runId);
      if (!target || target.terminal || pending.has(runId)) return;
      pending.add(runId);
      markInteracted();

      void execute(runId)
        .then((response) => {
          if (isCancelled()) return;
          commitIfCurrent(response, target.viewRevision);
          onSettled();
        })
        .catch(async (error: unknown) => {
          if (isCancelled()) return;
          let superseded = false;
          try {
            superseded = await revalidateTerminal(runId);
          } catch {
            // Revalidation is evidence only. Its failure must neither replace
            // nor hide the command failure the caller can still act on.
          }
          if (isCancelled()) return;
          if (superseded) {
            onSettled();
            return;
          }
          onFailure(runId, error);
        })
        .finally(() => pending.delete(runId));
    },
  };
}
