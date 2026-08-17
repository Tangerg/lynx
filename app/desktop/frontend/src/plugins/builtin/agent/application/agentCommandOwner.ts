import { RetirableTaskCohort } from "@/lib/taskQueue";

export class AgentCommandRetiredError extends Error {
  override readonly name = "AgentCommandRetiredError";

  constructor() {
    super("agent_command_owner_retired");
  }
}

interface SessionSummaryQueue {
  tail: Promise<void>;
  revision: number | null;
}

export interface SessionRollbackLease {
  isCurrent(): boolean;
  release(): void;
}

export interface AgentCommandEffect {
  /** The authoritative command settled; keep the local effect. */
  settle(): void;
  /** The command failed or its generation retired; compensate the local effect. */
  rollback(): void;
}

/**
 * Exact Plugin Host generation that owns Agent product commands and their local effects.
 *
 * Transport mutation identity answers whether one Runtime command committed. This owner
 * answers the outer product question: which installed Host may start that command, join
 * its single-flight work, or publish its response into navigation/query/material state.
 * A successor synchronously retires the predecessor before its gateway is published.
 */
export class AgentCommandOwner {
  static #active: AgentCommandOwner | null = null;

  readonly #creates = new Map<string, Promise<unknown>>();
  readonly #forks = new Map<string, Promise<unknown>>();
  readonly #rollbackSessions = new Set<string>();
  readonly #sessionSummaryQueues = new Map<string, SessionSummaryQueue>();
  readonly #effects = new Set<AgentCommandEffect>();
  readonly #retiredError = new AgentCommandRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  #approvalModeTail: Promise<void> = Promise.resolve();
  #approvalRulesTail: Promise<void> = Promise.resolve();

  private constructor() {}

  static install(): AgentCommandOwner {
    const predecessor = AgentCommandOwner.#active;
    const owner = new AgentCommandOwner();
    AgentCommandOwner.#active = owner;
    if (predecessor) predecessor.#retire();
    return owner;
  }

  static current(): AgentCommandOwner {
    const owner = AgentCommandOwner.#active;
    if (!owner) throw new AgentCommandRetiredError();
    return owner;
  }

  isCurrent(): boolean {
    return !this.#cohort.retired && AgentCommandOwner.#active === this;
  }

  assertCurrent(): void {
    if (AgentCommandOwner.#active !== this) throw this.#retiredError;
    this.#cohort.assertCurrent();
  }

  settle<T>(operation: Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#cohort.settle(operation);
  }

  runSessionCreate<T>(key: string | null, execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    if (key === null) return execute();
    return this.#runSingleFlight(this.#creates, key, execute);
  }

  runSessionFork<T>(key: string, execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#runSingleFlight(this.#forks, key, execute);
  }

  beginSessionRollback(sessionId: string): SessionRollbackLease | null {
    this.assertCurrent();
    if (this.#rollbackSessions.has(sessionId)) return null;
    this.#rollbackSessions.add(sessionId);
    let released = false;
    return {
      isCurrent: () => this.isCurrent(),
      release: () => {
        if (released) return;
        released = true;
        this.#rollbackSessions.delete(sessionId);
      },
    };
  }

  settleSessionSummary<T extends { revision: number }>(
    sessionId: string,
    expectedRevision: number,
    execute: (revision: number) => Promise<T>,
  ): Promise<T> {
    this.assertCurrent();
    const queue = this.#sessionSummaryQueues.get(sessionId) ?? {
      tail: Promise.resolve(),
      revision: null,
    };
    this.#sessionSummaryQueues.set(sessionId, queue);

    const result = queue.tail.then(() => {
      this.assertCurrent();
      return execute(Math.max(expectedRevision, queue.revision ?? expectedRevision));
    });
    const settled = result.then(
      (value) => {
        if (this.isCurrent()) queue.revision = value.revision;
      },
      () => undefined,
    );
    queue.tail = settled;
    void settled.finally(() => {
      if (this.#sessionSummaryQueues.get(sessionId)?.tail === settled) {
        this.#sessionSummaryQueues.delete(sessionId);
      }
    });
    return result;
  }

  serializeApprovalMode<T>(execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    const result = this.#approvalModeTail.then(() => {
      this.assertCurrent();
      return this.settle(execute());
    });
    this.#approvalModeTail = result.then(
      () => undefined,
      () => undefined,
    );
    return result;
  }

  serializeApprovalRules<T>(execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    const result = this.#approvalRulesTail.then(() => {
      this.assertCurrent();
      return this.settle(execute());
    });
    this.#approvalRulesTail = result.then(
      () => undefined,
      () => undefined,
    );
    return result;
  }

  trackEffect(rollback: () => void): AgentCommandEffect {
    this.assertCurrent();
    let pending = true;
    const effect: AgentCommandEffect = {
      settle: () => {
        if (!pending) return;
        pending = false;
        this.#effects.delete(effect);
      },
      rollback: () => {
        if (!pending) return;
        pending = false;
        this.#effects.delete(effect);
        rollback();
      },
    };
    this.#effects.add(effect);
    return effect;
  }

  dispose(): void {
    if (this.#cohort.retired) return;
    if (AgentCommandOwner.#active === this) AgentCommandOwner.#active = null;
    this.#retire();
  }

  #runSingleFlight<T>(
    flights: Map<string, Promise<unknown>>,
    key: string,
    execute: () => Promise<T>,
  ): Promise<T> {
    const existing = flights.get(key) as Promise<T> | undefined;
    if (existing) return existing;
    const operation = execute();
    const tracked = operation.finally(() => {
      if (flights.get(key) === tracked) flights.delete(key);
    });
    flights.set(key, tracked);
    return tracked;
  }

  #retire(): void {
    if (this.#cohort.retired) return;
    this.#cohort.retire();
    this.#creates.clear();
    this.#forks.clear();
    this.#rollbackSessions.clear();
    this.#sessionSummaryQueues.clear();
    this.#approvalModeTail = Promise.resolve();
    this.#approvalRulesTail = Promise.resolve();
    for (const effect of [...this.#effects]) effect.rollback();
  }
}

export function agentCommandOwner(): AgentCommandOwner {
  return AgentCommandOwner.current();
}

export function agentCommandWasRetired(error: unknown): boolean {
  return error instanceof AgentCommandRetiredError;
}
