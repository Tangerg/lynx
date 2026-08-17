import type {
  AgentSessionViewEntry,
  AgentSessionViewPort,
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
  superseded: boolean;
}

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

class InterruptResponseCoordinator {
  readonly #view: AgentSessionViewPort;
  readonly #batches = new Map<string, ResponseBatch>();
  readonly #unsubscribe: () => void;
  #retired = false;

  constructor(view: AgentSessionViewPort) {
    this.#view = view;
    this.#unsubscribe = view.subscribeSessions((sessions) => this.#reconcile(sessions));
  }

  stage(
    sessionId: string,
    rootRunId: string,
    itemId: string,
    response: InterruptResumeInput["response"],
    settled: ResolvePatch,
    hooks?: { onSettled?: () => void; onError?: () => void },
  ): boolean {
    if (this.#retired) return false;
    const entry = this.#view.getSession(sessionId);
    if (!entry?.resume) return false;
    const expected = openResponseIds(entry, rootRunId);
    if (!expected.includes(itemId)) return false;

    const key = batchKey(sessionId, rootRunId);
    let batch = this.#batches.get(key);
    if (!batch) {
      batch = {
        sessionId,
        rootRunId,
        responses: new Map(),
        submitting: false,
        superseded: false,
      };
      this.#batches.set(key, batch);
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
        if (this.#batches.get(key) !== batch) return;
        this.#batches.delete(key);
        const resolvedAt = Date.now();
        for (const staged of ordered) {
          this.#view.resolveInterrupt(sessionId, staged.input.itemId, staged.settled, resolvedAt);
          staged.onSettled?.();
        }
      },
      () => {
        const superseded = batch!.superseded;
        this.#reject(batch!);
        return superseded;
      },
    );
    if (!accepted && this.#batches.get(key) === batch) this.#reject(batch);
    return true;
  }

  isStaged(sessionId: string, rootRunId: string, itemId: string): boolean {
    return this.#batches.get(batchKey(sessionId, rootRunId))?.responses.has(itemId) ?? false;
  }

  discard(): void {
    for (const batch of [...this.#batches.values()]) this.#reject(batch);
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#unsubscribe();
    this.discard();
  }

  #reject(batch: ResponseBatch, superseded = false): void {
    const key = batchKey(batch.sessionId, batch.rootRunId);
    if (this.#batches.get(key) !== batch) return;
    batch.superseded ||= superseded;
    this.#batches.delete(key);
    for (const response of batch.responses.values()) response.onError?.();
  }

  #reconcile(sessions: Record<string, AgentSessionViewEntry>): void {
    if (this.#retired) return;
    for (const batch of [...this.#batches.values()]) {
      const entry = sessions[batch.sessionId];
      const open = entry ? new Set(openResponseIds(entry, batch.rootRunId)) : new Set<string>();
      if ([...batch.responses.keys()].some((itemId) => !open.has(itemId))) {
        // A submitted set disappearing is an authoritative continuation
        // boundary. Its later rejection is stale, not a new command failure.
        this.#reject(batch, batch.submitting);
      }
    }
  }
}

let activeCoordinator: InterruptResponseCoordinator | null = null;

function coordinator(): InterruptResponseCoordinator {
  if (!activeCoordinator) throw new Error("Interrupt response coordinator is not installed");
  return activeCoordinator;
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
  return coordinator().stage(sessionId, rootRunId, itemId, response, settled, hooks);
}

export function interruptResponseIsStaged(
  sessionId: string,
  rootRunId: string,
  itemId: string,
): boolean {
  return coordinator().isStaged(sessionId, rootRunId, itemId);
}

/** Own staged choices, continuation settlement and projection reconciliation
 * for one Agent Plugin Host generation. */
export function installInterruptResponseCoordinator(): () => void {
  const next = new InterruptResponseCoordinator(agentSessionView());
  activeCoordinator?.retire();
  activeCoordinator = next;
  return () => {
    if (activeCoordinator === next) activeCoordinator = null;
    next.retire();
  };
}

/** Test seam for hook tests which configure the Agent ports once per process. */
export function discardStagedInterruptResponses(): void {
  activeCoordinator?.discard();
}
