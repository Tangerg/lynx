import { RetirableTaskCohort } from "@/lib/taskQueue";

interface RunCancellationTarget {
  terminal: boolean;
  viewEpoch: number;
  viewRevision: number;
}

interface RunCancellationControllerOptions<Response> {
  markInteracted: () => void;
  readTarget: (runId: string) => RunCancellationTarget | null;
  execute: (runId: string) => Promise<Response>;
  commitIfCurrent: (response: Response, target: RunCancellationTarget) => boolean;
  revalidateTerminal: (runId: string) => Promise<boolean>;
  onSettled: () => void;
  onFailure: (runId: string, error: unknown) => void;
}

export interface RunCancellationController {
  cancel(runId: string): void;
  retire(): void;
}

class RunCancellationGenerationRetiredError extends Error {
  override readonly name = "RunCancellationGenerationRetiredError";

  constructor() {
    super("run_cancellation_generation_retired");
  }
}

/** Own one cancellation command per Run inside one replaceable Runtime generation.
 *
 * A successful response is a snapshot captured at command commit time, so it
 * may only fold while the material view still has the event epoch and revision
 * from which the command started. Retirement settles admitted work immediately;
 * a failed current-generation command is revalidated through the neutral Agent
 * projection, where another client reaching terminal is objective success and
 * an active authoritative Run preserves the original command failure.
 */
export function createRunCancellationController<Response>({
  markInteracted,
  readTarget,
  execute,
  commitIfCurrent,
  revalidateTerminal,
  onSettled,
  onFailure,
}: RunCancellationControllerOptions<Response>): RunCancellationController {
  const pending = new Set<string>();
  const retiredError = new RunCancellationGenerationRetiredError();
  const cohort = new RetirableTaskCohort(retiredError);

  return {
    cancel(runId) {
      if (cohort.retired) return;
      const target = readTarget(runId);
      if (!target || target.terminal || pending.has(runId)) return;
      pending.add(runId);
      markInteracted();

      let command: Promise<Response>;
      try {
        command = execute(runId);
      } catch (error) {
        command = Promise.reject(error);
      }
      void cohort
        .settle(command)
        .then((response) => {
          commitIfCurrent(response, target);
          onSettled();
        })
        .catch(async (error: unknown) => {
          if (error === retiredError) return;
          let superseded = false;
          try {
            superseded = await cohort.settle(revalidateTerminal(runId));
          } catch (revalidationError) {
            if (revalidationError === retiredError) return;
            // Revalidation is evidence only. Its failure must neither replace
            // nor hide the command failure the caller can still act on.
          }
          if (superseded) {
            onSettled();
            return;
          }
          onFailure(runId, error);
        })
        .finally(() => pending.delete(runId));
    },
    retire() {
      cohort.retire();
    },
  };
}
