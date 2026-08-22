import { createPublicationSlot } from "@/lib/publicationSlot";
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

interface ProjectionBoundary {
  generation: number;
  authoritativeRevision: number;
}

interface SubmittedResponseBatch {
  openedAt: ProjectionBoundary;
  awaitingAuthority: ProjectionBoundary | null;
  superseded: boolean;
}

class InterruptResponseBatch {
  readonly #responses = new Map<string, StagedResponse>();
  #submission: SubmittedResponseBatch | null = null;

  constructor(
    readonly sessionId: string,
    readonly rootRunId: string,
  ) {}

  get submitting(): boolean {
    return this.#submission !== null;
  }

  stage(itemId: string, response: StagedResponse): void {
    this.#responses.get(itemId)?.onError?.();
    this.#responses.set(itemId, response);
  }

  contains(itemId: string): boolean {
    return this.#responses.has(itemId);
  }

  covers(itemIds: readonly string[]): boolean {
    return itemIds.every((itemId) => this.#responses.has(itemId));
  }

  ordered(itemIds: readonly string[]): StagedResponse[] {
    return itemIds.map((itemId) => this.#responses.get(itemId)!);
  }

  submit(entry: AgentSessionViewEntry): void {
    this.#submission = {
      openedAt: this.#boundary(entry),
      awaitingAuthority: null,
      superseded: false,
    };
  }

  responses(): IterableIterator<StagedResponse> {
    return this.#responses.values();
  }

  openingRejectionIsStale(): boolean {
    return this.#submission?.superseded ?? false;
  }

  /** Reconcile an in-flight command against one material generation.
   *
   * Retirement alone is not a settlement fact: keep cards latched across the
   * disconnected gap. Once a durable projection commits in that successor
   * generation, either the barrier disappeared (the command committed) or the
   * exact barrier remains (the old command no longer owns it and is retryable).
   */
  reconcile(entry: AgentSessionViewEntry | undefined): boolean {
    const open = entry ? new Set(openResponseIds(entry, this.rootRunId)) : new Set<string>();
    if ([...this.#responses.keys()].some((itemId) => !open.has(itemId))) {
      if (this.#submission) this.#submission.superseded = true;
      return true;
    }

    const submission = this.#submission;
    if (!submission || !entry || entry.viewEpoch === submission.openedAt.generation) return false;

    const current = this.#boundary(entry);
    if (submission.awaitingAuthority?.generation !== current.generation) {
      submission.awaitingAuthority = current;
      return false;
    }
    return current.authoritativeRevision > submission.awaitingAuthority.authoritativeRevision;
  }

  #boundary(entry: AgentSessionViewEntry): ProjectionBoundary {
    return {
      generation: entry.viewEpoch,
      authoritativeRevision: entry.authoritativeRevision,
    };
  }
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
  readonly #batches = new Map<string, InterruptResponseBatch>();
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
      batch = new InterruptResponseBatch(sessionId, rootRunId);
      this.#batches.set(key, batch);
    }
    if (batch.submitting) return false;

    batch.stage(itemId, {
      input: { itemId, response },
      settled,
      ...hooks,
    });

    if (!batch.covers(expected)) return true;
    batch.submit(entry);
    const ordered = batch.ordered(expected);
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
        const superseded = batch.openingRejectionIsStale();
        this.#reject(batch);
        return superseded;
      },
    );
    if (!accepted && this.#batches.get(key) === batch) this.#reject(batch);
    return true;
  }

  isStaged(sessionId: string, rootRunId: string, itemId: string): boolean {
    return this.#batches.get(batchKey(sessionId, rootRunId))?.contains(itemId) ?? false;
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

  #reject(batch: InterruptResponseBatch): void {
    const key = batchKey(batch.sessionId, batch.rootRunId);
    if (this.#batches.get(key) !== batch) return;
    this.#batches.delete(key);
    for (const response of batch.responses()) response.onError?.();
  }

  #reconcile(sessions: Record<string, AgentSessionViewEntry>): void {
    if (this.#retired) return;
    for (const batch of [...this.#batches.values()]) {
      const entry = sessions[batch.sessionId];
      if (batch.reconcile(entry)) this.#reject(batch);
    }
  }
}

const interruptResponsePublication = createPublicationSlot<InterruptResponseCoordinator>();

function coordinator(): InterruptResponseCoordinator {
  const current = interruptResponsePublication.current();
  if (!current) throw new Error("Interrupt response coordinator is not installed");
  return current;
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
  interruptResponsePublication.publish(next, (predecessor) => predecessor.retire());
  return () => {
    interruptResponsePublication.withdraw(next);
    next.retire();
  };
}
