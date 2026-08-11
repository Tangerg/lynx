import type {
  AgentSessionViewEntry,
  InterruptResumeInput,
  ResolvePatch,
} from "../ports/sessionView";
import { agentSessionView } from "../ports/sessionView";

interface StagedResponse {
  input: InterruptResumeInput;
  settled: ResolvePatch;
  onSettled?: () => void;
  onError?: () => void;
}

interface ResponseBatch {
  sessionId: string;
  rootRunId: string;
  responses: Map<string, StagedResponse>;
  submitting: boolean;
}

const batches = new Map<string, ResponseBatch>();

const batchKey = (sessionId: string, rootRunId: string) => `${sessionId}\u0000${rootRunId}`;

function openResponseIds(entry: AgentSessionViewEntry, rootRunId: string): string[] {
  return entry.view.pendingInterrupts
    .filter((group) => group.rootRunId === rootRunId)
    .flatMap((group) =>
      group.interrupts
        .filter((interrupt) => interrupt.kind === "approval" || interrupt.kind === "question")
        .map((interrupt) => interrupt.itemId),
    );
}

function rejectBatch(batch: ResponseBatch): void {
  batches.delete(batchKey(batch.sessionId, batch.rootRunId));
  for (const response of batch.responses.values()) response.onError?.();
}

/**
 * Stage one person's answer to a Runtime pending set.
 *
 * A pending set is one atomic resume barrier, even when its cards came from
 * different child runs. The UI may collect those decisions one card at a time,
 * but the Runtime command is opened only after every answerable item has a
 * response, and it always addresses the owning root Run.
 */
export function stageInterruptResponse(
  sessionId: string,
  rootRunId: string,
  itemId: string,
  response: InterruptResumeInput["response"],
  settled: ResolvePatch,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  const entry = agentSessionView().getSession(sessionId);
  if (!entry?.resume) return false;
  const expected = openResponseIds(entry, rootRunId);
  if (!expected.includes(itemId)) return false;

  const key = batchKey(sessionId, rootRunId);
  let batch = batches.get(key);
  if (!batch) {
    batch = { sessionId, rootRunId, responses: new Map(), submitting: false };
    batches.set(key, batch);
  }
  if (batch.submitting) return false;

  const previous = batch.responses.get(itemId);
  if (previous) previous.onError?.();
  batch.responses.set(itemId, {
    input: { itemId, response },
    settled,
    ...hooks,
  });

  if (!expected.every((id) => batch!.responses.has(id))) return true;
  batch.submitting = true;
  const ordered = expected.map((id) => batch!.responses.get(id)!);
  const accepted = entry.resume(
    rootRunId,
    ordered.map(({ input }) => input),
    () => {
      batches.delete(key);
      const resolvedAt = Date.now();
      for (const staged of ordered) {
        agentSessionView().resolveInterrupt(
          sessionId,
          staged.input.itemId,
          staged.settled,
          resolvedAt,
        );
        staged.onSettled?.();
      }
    },
    () => rejectBatch(batch!),
  );
  // Another local command may own the single run-opening channel. A refused
  // handoff has no asynchronous callback, so release every staged card here.
  // Guard the batch identity because a test or adapter may invoke a callback
  // synchronously before returning.
  if (!accepted && batches.get(key) === batch) rejectBatch(batch);
  return true;
}

export function interruptResponseIsStaged(
  sessionId: string,
  rootRunId: string,
  itemId: string,
): boolean {
  return batches.get(batchKey(sessionId, rootRunId))?.responses.has(itemId) ?? false;
}

function reconcileBatches(sessions: Record<string, AgentSessionViewEntry>): void {
  for (const batch of [...batches.values()]) {
    if (batch.submitting) continue;
    const entry = sessions[batch.sessionId];
    const open = entry ? new Set(openResponseIds(entry, batch.rootRunId)) : new Set<string>();
    if ([...batch.responses.keys()].some((itemId) => !open.has(itemId))) rejectBatch(batch);
  }
}

/** Keep card-local staged decisions aligned with authoritative projection
 * replacements and session teardown. */
export function installInterruptResponseReconciliation(): () => void {
  const view = agentSessionView();
  const unsubscribe = view.subscribeSessions(reconcileBatches);
  return () => {
    unsubscribe();
    for (const batch of [...batches.values()]) rejectBatch(batch);
  };
}

/** Test seam for hook tests which configure the Agent ports once per process. */
export function discardStagedInterruptResponses(): void {
  for (const batch of [...batches.values()]) rejectBatch(batch);
}
