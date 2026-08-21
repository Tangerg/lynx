import {
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
  openDurableMutationJournal,
  type DurableMutationIdentity,
  type DurableMutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
} from "./durableMutationJournal";
import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

export {
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
} from "./durableMutationJournal";
export type { MutationJournalScope, MutationJournalStorage } from "./durableMutationJournal";

export interface MutationReservation {
  readonly idempotencyKey: string;
  authorizeAttempt(): string;
  track<T>(mutation: MutationPromise<T>): MutationPromise<T>;
}

export interface MutationJournal {
  reserve(method: string, params: unknown, preferredKey?: string): MutationReservation | undefined;
  dispose(): void;
}

export interface MutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now?: () => number;
}

function rejectedMutation<T>(
  error: unknown,
  idempotencyKey: string,
  retry: (options?: { signal?: AbortSignal }) => MutationPromise<T>,
): MutationPromise<T> {
  const rejected = Promise.reject(error);
  return Object.defineProperties(rejected, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: { enumerable: true, value: retry },
  }) as unknown as MutationPromise<T>;
}

interface MutationLifecycle {
  begin(): void;
  claim(): void;
  resolve(): void;
  reject(error: unknown): void;
}

function trackedMutation<T>(
  mutation: MutationPromise<T>,
  lifecycle: MutationLifecycle,
): MutationPromise<T> {
  lifecycle.begin();
  const tracked = mutation.then(
    (value) => {
      lifecycle.resolve();
      return value;
    },
    (error: unknown) => {
      lifecycle.reject(error);
      throw error;
    },
  );
  const retry = (options?: { signal?: AbortSignal }): MutationPromise<T> => {
    try {
      lifecycle.claim();
      return trackedMutation(mutation.retry(options), lifecycle);
    } catch (error) {
      return rejectedMutation(error, mutation.idempotencyKey, retry);
    }
  };
  return Object.defineProperties(tracked, {
    idempotencyKey: { enumerable: true, get: () => mutation.idempotencyKey },
    retry: { enumerable: true, value: retry },
  }) as MutationPromise<T>;
}

let currentAuthority: RendererMutationAuthority | null = null;

/** Owns only the current renderer's right to deliver and settle commands.
 * Durable command identity remains in DurableMutationJournal. */
class RendererMutationAuthority implements MutationJournal {
  readonly #journal: DurableMutationJournal;
  readonly #claims = new Set<string>();
  #retired = false;

  constructor(journal: DurableMutationJournal) {
    this.#journal = journal;
  }

  reserve(method: string, params: unknown, preferredKey?: string): MutationReservation | undefined {
    this.#assertCurrent();
    const identity = this.#journal.reserve(method, params, preferredKey, (idempotencyKey) =>
      this.#claims.has(idempotencyKey),
    );
    if (!identity) return undefined;
    this.#claims.add(identity.entry.idempotencyKey);
    const lifecycle = this.#lifecycle(identity);
    return {
      idempotencyKey: identity.entry.idempotencyKey,
      authorizeAttempt: () => {
        this.#assertClaim(identity);
        return this.#journal.authorize(identity);
      },
      track: (mutation) => trackedMutation(mutation, lifecycle),
    };
  }

  dispose(): void {
    if (this.#retired) return;
    if (currentAuthority === this) currentAuthority = null;
    this.retireForReplacement();
  }

  retireForReplacement(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#claims.clear();
  }

  #lifecycle(identity: DurableMutationIdentity): MutationLifecycle {
    const idempotencyKey = identity.entry.idempotencyKey;
    let activeAttempts = 0;
    let definitiveOutcome = false;
    const finish = (definitive: boolean) => {
      activeAttempts = Math.max(0, activeAttempts - 1);
      this.#assertClaim(identity);
      definitiveOutcome ||= definitive;
      if (activeAttempts > 0) return;
      if (definitiveOutcome) {
        this.#journal.settle(identity);
        definitiveOutcome = false;
      }
      this.#claims.delete(idempotencyKey);
    };
    return {
      begin: () => {
        this.#assertClaim(identity);
        activeAttempts += 1;
      },
      claim: () => {
        this.#assertCurrent();
        if (activeAttempts === 0) definitiveOutcome = false;
        if (!this.#claims.has(idempotencyKey)) {
          this.#journal.retain(identity);
          this.#claims.add(idempotencyKey);
        }
      },
      resolve: () => finish(true),
      reject: (error) =>
        finish(
          !mutationSettlementIsUnknown(error) &&
            !(error instanceof MutationJournalStorageError) &&
            !(error instanceof MutationJournalScopeUnavailableError),
        ),
    };
  }

  #assertClaim(identity: DurableMutationIdentity): void {
    this.#assertCurrent();
    if (!this.#claims.has(identity.entry.idempotencyKey)) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity is no longer owned by this renderer",
      );
    }
  }

  #assertCurrent(): void {
    if (this.#retired || currentAuthority !== this) {
      throw new MutationJournalOwnershipError("Runtime mutation renderer generation was replaced");
    }
  }
}

/**
 * Retain unresolved mutation identities across renderer and Runtime restarts.
 *
 * Construction publishes the successor renderer authority before retiring its
 * predecessor. No owner, lease, heartbeat, or renderer generation is persisted:
 * the durable journal answers only which command identity remains unresolved.
 */
export function createMutationJournal(options: MutationJournalOptions): MutationJournal {
  const successor = new RendererMutationAuthority(
    openDurableMutationJournal({ ...options, now: options.now ?? Date.now }),
  );
  const predecessor = currentAuthority;
  currentAuthority = successor;
  predecessor?.retireForReplacement();
  return successor;
}
