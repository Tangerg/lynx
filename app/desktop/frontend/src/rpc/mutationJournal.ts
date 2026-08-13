import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

const JOURNAL_VERSION = 2;
const LEGACY_JOURNAL_VERSION = 1;
const OWNER_RECORD_VERSION = 1;
const MAX_ENTRIES = 256;
const OWNER_LEASE_MS = 30_000;
const OWNER_HEARTBEAT_MS = 10_000;
const ENTRY_PREFIX = "entry:";
const OWNER_PREFIX = "owner:";
const LEGACY_KEY = "legacy:v1";
const PROCESS_OWNER = crypto.randomUUID();
const PROCESS_STARTED_AT =
  typeof performance !== "undefined" && Number.isFinite(performance.timeOrigin)
    ? performance.timeOrigin
    : Date.now();
const ACTIVE_PROCESS_CLAIMS = new WeakMap<MutationJournalStorage, Map<string, string>>();
const PROCESS_CLAIMABLE = new Set<string>();

export interface MutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}

export interface MutationJournalScope {
  /** Normalized transport endpoint. An identical Runtime store reached through a
   * different configured endpoint is deliberately a different client scope. */
  endpoint: string;
  /** Opaque identity published by the Runtime's durable idempotency store. */
  namespace: string;
  retentionSeconds: number;
}

export interface MutationReservation {
  readonly idempotencyKey: string;
  track<T>(mutation: MutationPromise<T>): MutationPromise<T>;
}

export interface MutationJournal {
  /** Reserve a durable identity. `preferredKey` is an opaque candidate owned by
   * this invocation; it lets a retry recover a write whose confirmation failed
   * without making that unconfirmed record claimable by a same-shaped twin. */
  reserve(method: string, params: unknown, preferredKey?: string): MutationReservation | undefined;
  dispose(): void;
}

export interface MutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now?: () => number;
}

export class MutationJournalError extends Error {
  override readonly name: string = "MutationJournalError";
}

export class MutationJournalStorageError extends MutationJournalError {
  override readonly name = "MutationJournalStorageError";
}

export class MutationJournalOwnershipError extends MutationJournalError {
  override readonly name = "MutationJournalOwnershipError";
}

export class MutationJournalCapacityError extends MutationJournalError {
  override readonly name = "MutationJournalCapacityError";
}

function activeProcessClaims(storage: MutationJournalStorage): Map<string, string> {
  let claims = ACTIVE_PROCESS_CLAIMS.get(storage);
  if (!claims) {
    claims = new Map();
    ACTIVE_PROCESS_CLAIMS.set(storage, claims);
  }
  return claims;
}

interface JournalEntry {
  version: typeof JOURNAL_VERSION;
  salt: string;
  endpoint: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  owner: string;
  claimable: boolean;
  createdAt: number;
  expiresAt: number;
}

interface LegacyJournalEntry {
  endpoint: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  owner: string;
  claimable: boolean;
  createdAt: number;
  expiresAt: number;
}

interface LegacyJournalSnapshot {
  version: typeof LEGACY_JOURNAL_VERSION;
  salt: string;
  entries: LegacyJournalEntry[];
}

interface OwnerLease {
  version: typeof OWNER_RECORD_VERSION;
  owner: string;
  startedAt: number;
  expiresAt: number;
}

function journalError(message: string, cause?: unknown): MutationJournalStorageError {
  if (cause instanceof MutationJournalStorageError) return cause;
  return new MutationJournalStorageError(message, cause === undefined ? undefined : { cause });
}

function validText(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= maximum;
}

function validLegacyEntry(value: unknown): value is LegacyJournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<LegacyJournalEntry>;
  return (
    validText(entry.endpoint, 2_048) &&
    validText(entry.namespace, 255) &&
    typeof entry.fingerprint === "string" &&
    /^[0-9a-f]{32}$/.test(entry.fingerprint) &&
    validText(entry.idempotencyKey, 255) &&
    validText(entry.owner, 255) &&
    typeof entry.claimable === "boolean" &&
    typeof entry.createdAt === "number" &&
    Number.isFinite(entry.createdAt) &&
    typeof entry.expiresAt === "number" &&
    Number.isFinite(entry.expiresAt) &&
    entry.expiresAt > entry.createdAt
  );
}

function validEntry(value: unknown): value is JournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<JournalEntry>;
  return entry.version === JOURNAL_VERSION && validText(entry.salt, 255) && validLegacyEntry(entry);
}

function validOwnerLease(value: unknown): value is OwnerLease {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const lease = value as Partial<OwnerLease>;
  return (
    lease.version === OWNER_RECORD_VERSION &&
    validText(lease.owner, 255) &&
    typeof lease.startedAt === "number" &&
    Number.isFinite(lease.startedAt) &&
    typeof lease.expiresAt === "number" &&
    Number.isFinite(lease.expiresAt)
  );
}

function entryKey(idempotencyKey: string): string {
  return `${ENTRY_PREFIX}${encodeURIComponent(idempotencyKey)}`;
}

function ownerKey(owner: string): string {
  return `${OWNER_PREFIX}${encodeURIComponent(owner)}`;
}

function normalizedEndpoint(endpoint: string): string {
  return endpoint.replace(/\/+$/, "");
}

function validScope(scope: MutationJournalScope | null | undefined): scope is MutationJournalScope {
  if (!scope) return false;
  const endpoint = normalizedEndpoint(scope.endpoint);
  return (
    endpoint.length > 0 &&
    endpoint.length <= 2_048 &&
    validText(scope.namespace, 255) &&
    Number.isInteger(scope.retentionSeconds) &&
    scope.retentionSeconds > 0
  );
}

function readKeys(storage: MutationJournalStorage): string[] {
  try {
    const keys = storage.keys();
    if (!Array.isArray(keys) || keys.some((key) => typeof key !== "string")) {
      throw new TypeError("storage returned invalid keys");
    }
    return [...new Set(keys)];
  } catch (error) {
    throw journalError("Runtime mutation journal keys are unavailable", error);
  }
}

function readValue(storage: MutationJournalStorage, key: string): unknown {
  try {
    return storage.get(key);
  } catch (error) {
    throw journalError(`Runtime mutation journal record is unreadable: ${key}`, error);
  }
}

function persistValue(storage: MutationJournalStorage, key: string, value: unknown): void {
  try {
    storage.set(key, value);
  } catch (error) {
    throw journalError(`Runtime mutation journal record was not persisted: ${key}`, error);
  }
}

function removeValue(storage: MutationJournalStorage, key: string): void {
  try {
    storage.remove(key);
  } catch (error) {
    throw journalError(`Runtime mutation journal record was not removed: ${key}`, error);
  }
}

function loadEntries(storage: MutationJournalStorage): JournalEntry[] {
  return readKeys(storage)
    .filter((key) => key.startsWith(ENTRY_PREFIX))
    .map((key) => {
      const value = readValue(storage, key);
      if (!validEntry(value) || entryKey(value.idempotencyKey) !== key) {
        throw journalError(`Runtime mutation journal entry is corrupted: ${key}`);
      }
      return value;
    });
}

function loadOwnerLeases(storage: MutationJournalStorage, currentTime: number): OwnerLease[] {
  const active: OwnerLease[] = [];
  for (const key of readKeys(storage).filter((candidate) => candidate.startsWith(OWNER_PREFIX))) {
    const value = readValue(storage, key);
    if (!validOwnerLease(value) || ownerKey(value.owner) !== key) {
      throw journalError(`Runtime mutation journal owner is corrupted: ${key}`);
    }
    if (value.expiresAt <= currentTime) {
      removeValue(storage, key);
    } else {
      active.push(value);
    }
  }
  return active;
}

function sameMigratedEntry(
  current: JournalEntry,
  legacy: LegacyJournalEntry,
  salt: string,
): boolean {
  return (
    current.salt === salt &&
    current.endpoint === legacy.endpoint &&
    current.namespace === legacy.namespace &&
    current.fingerprint === legacy.fingerprint &&
    current.idempotencyKey === legacy.idempotencyKey &&
    current.createdAt === legacy.createdAt &&
    current.expiresAt === legacy.expiresAt
  );
}

function migrateLegacySnapshot(storage: MutationJournalStorage): void {
  const value = readValue(storage, LEGACY_KEY);
  if (value === undefined) return;
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw journalError("Legacy Runtime mutation journal is corrupted");
  }
  const snapshot = value as Partial<LegacyJournalSnapshot>;
  if (
    snapshot.version !== LEGACY_JOURNAL_VERSION ||
    !validText(snapshot.salt, 255) ||
    !Array.isArray(snapshot.entries) ||
    snapshot.entries.length > MAX_ENTRIES ||
    !snapshot.entries.every(validLegacyEntry)
  ) {
    throw journalError("Legacy Runtime mutation journal is corrupted");
  }

  for (const legacy of snapshot.entries) {
    const key = entryKey(legacy.idempotencyKey);
    const existing = readValue(storage, key);
    if (existing !== undefined) {
      if (!validEntry(existing) || !sameMigratedEntry(existing, legacy, snapshot.salt)) {
        throw journalError(`Legacy Runtime mutation journal conflicts with ${key}`);
      }
      continue;
    }
    persistValue(storage, key, {
      version: JOURNAL_VERSION,
      salt: snapshot.salt,
      ...legacy,
    } satisfies JournalEntry);
  }
  removeValue(storage, LEGACY_KEY);
}

/** Canonical JSON is used only as transient hash input. Request bodies, prompts,
 * credentials, and file contents are never written to persistent storage. */
function canonicalJSON(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value);
  if (typeof value === "number") return Number.isFinite(value) ? JSON.stringify(value) : "null";
  if (Array.isArray(value)) {
    return `[${value
      .map((item) =>
        item === undefined || typeof item === "function" || typeof item === "symbol"
          ? "null"
          : canonicalJSON(item),
      )
      .join(",")}]`;
  }
  if (typeof value === "object") {
    const record = value as Record<string, unknown>;
    const fields = Object.keys(record)
      .filter((key) => {
        const field = record[key];
        return field !== undefined && typeof field !== "function" && typeof field !== "symbol";
      })
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalJSON(record[key])}`);
    return `{${fields.join(",")}}`;
  }
  return "null";
}

/** A compact 128-bit matching fingerprint. It is not an authorization or
 * integrity primitive: Runtime still hashes the real method/params and rejects a
 * collision as idempotency_conflict. The journal needs only a privacy-preserving
 * lookup key that is stable across Desktop restarts. */
function fingerprint(value: string): string {
  let h1 = 1_779_033_703;
  let h2 = 3_144_134_277;
  let h3 = 1_013_904_242;
  let h4 = 2_773_480_762;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    h1 = h2 ^ Math.imul(h1 ^ code, 597_399_067);
    h2 = h3 ^ Math.imul(h2 ^ code, 2_869_860_233);
    h3 = h4 ^ Math.imul(h3 ^ code, 951_274_213);
    h4 = h1 ^ Math.imul(h4 ^ code, 2_716_044_179);
  }
  h1 = Math.imul(h3 ^ (h1 >>> 18), 597_399_067);
  h2 = Math.imul(h4 ^ (h2 >>> 22), 2_869_860_233);
  h3 = Math.imul(h1 ^ (h3 >>> 17), 951_274_213);
  h4 = Math.imul(h2 ^ (h4 >>> 19), 2_716_044_179);
  const hex = (part: number) => (part >>> 0).toString(16).padStart(8, "0");
  return hex(h1) + hex(h2) + hex(h3) + hex(h4);
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

/**
 * Retain unresolved mutation identities across Desktop process restarts.
 *
 * Each entry and owner lease is an independent verified record. That prevents
 * one renderer from overwriting another renderer's journal. An owner whose
 * lease may still be alive blocks a same-shaped command in another renderer;
 * after the lease expires, the oldest live renderer claims the original key.
 * The transport is never opened until that durable ownership transition has
 * succeeded.
 */
export function createMutationJournal(options: MutationJournalOptions): MutationJournal {
  const now = options.now ?? Date.now;
  const journalInstance = crypto.randomUUID();
  const activeClaims = activeProcessClaims(options.storage);
  const unconfirmedUnsent = new Set<string>();
  let heartbeat: ReturnType<typeof setInterval> | undefined;
  let disposed = false;

  const assertOpen = () => {
    if (disposed) throw new MutationJournalError("Runtime mutation journal is closed");
  };

  const refreshOwner = (currentTime: number) => {
    assertOpen();
    persistValue(options.storage, ownerKey(PROCESS_OWNER), {
      version: OWNER_RECORD_VERSION,
      owner: PROCESS_OWNER,
      startedAt: PROCESS_STARTED_AT,
      expiresAt: currentTime + OWNER_LEASE_MS,
    } satisfies OwnerLease);
  };

  const startHeartbeat = () => {
    if (heartbeat !== undefined) return;
    heartbeat = setInterval(() => {
      try {
        refreshOwner(now());
      } catch {
        // reserve/claim verify durability synchronously before transport work.
        // A background heartbeat cannot surface an actionable Promise failure.
      }
    }, OWNER_HEARTBEAT_MS);
  };

  const prepare = (currentTime: number) => {
    assertOpen();
    migrateLegacySnapshot(options.storage);
    refreshOwner(currentTime);
  };

  const readEntry = (key: string): JournalEntry | undefined => {
    const value = readValue(options.storage, entryKey(key));
    if (value === undefined) return undefined;
    if (!validEntry(value) || value.idempotencyKey !== key) {
      throw journalError(`Runtime mutation journal entry is corrupted: ${entryKey(key)}`);
    }
    return value;
  };

  const persistEntry = (entry: JournalEntry) => {
    persistValue(options.storage, entryKey(entry.idempotencyKey), entry);
  };

  const removeEntry = (entry: JournalEntry) => {
    removeValue(options.storage, entryKey(entry.idempotencyKey));
    activeClaims.delete(entry.idempotencyKey);
    PROCESS_CLAIMABLE.delete(entry.idempotencyKey);
  };

  const lifecycleFor = (reserved: JournalEntry): MutationLifecycle => {
    let activeAttempts = 0;
    let definitiveOutcome = false;

    const finish = (definitive: boolean) => {
      activeAttempts = Math.max(0, activeAttempts - 1);
      definitiveOutcome ||= definitive;
      if (activeAttempts > 0) return;
      if (activeClaims.get(reserved.idempotencyKey) !== journalInstance) {
        definitiveOutcome = false;
        return;
      }

      const current = readEntry(reserved.idempotencyKey);
      if (current && current.owner !== PROCESS_OWNER) {
        activeClaims.delete(reserved.idempotencyKey);
        definitiveOutcome = false;
        return;
      }
      if (definitiveOutcome) {
        if (current) {
          try {
            removeEntry(current);
          } catch (error) {
            PROCESS_CLAIMABLE.add(reserved.idempotencyKey);
            throw error;
          }
        }
        definitiveOutcome = false;
        return;
      }
      if (!current) {
        PROCESS_CLAIMABLE.add(reserved.idempotencyKey);
        throw journalError("Runtime mutation identity disappeared before settlement");
      }
      const claimable = { ...current, owner: PROCESS_OWNER, claimable: true };
      try {
        persistEntry(claimable);
        PROCESS_CLAIMABLE.delete(reserved.idempotencyKey);
      } catch (error) {
        PROCESS_CLAIMABLE.add(reserved.idempotencyKey);
        throw error;
      }
    };

    return {
      begin: () => {
        activeAttempts += 1;
      },
      claim: () => {
        const currentTime = now();
        prepare(currentTime);
        const claimOwner = activeClaims.get(reserved.idempotencyKey);
        const current = readEntry(reserved.idempotencyKey);
        if (!current) {
          if (claimOwner !== undefined && claimOwner !== journalInstance) {
            throw new MutationJournalOwnershipError(
              "Runtime mutation identity is owned by a successor Desktop client",
            );
          }
          return;
        }
        if (claimOwner !== journalInstance || current.owner !== PROCESS_OWNER) {
          throw new MutationJournalOwnershipError(
            "Runtime mutation identity is owned by a successor Desktop client",
          );
        }
        if (current.expiresAt <= currentTime) {
          throw new MutationJournalError("Runtime mutation identity expired before retry");
        }
        persistEntry({ ...current, owner: PROCESS_OWNER, claimable: false });
        PROCESS_CLAIMABLE.delete(reserved.idempotencyKey);
        if (activeAttempts === 0) definitiveOutcome = false;
        startHeartbeat();
      },
      resolve: () => finish(true),
      reject: (error) => finish(!mutationSettlementIsUnknown(error)),
    };
  };

  return {
    reserve(method, params, preferredKey) {
      const scope = options.scope();
      if (!validScope(scope)) return undefined;
      if (preferredKey !== undefined && !validText(preferredKey, 255)) {
        throw new MutationJournalError("Runtime mutation identity candidate is invalid");
      }
      const endpoint = normalizedEndpoint(scope.endpoint);
      const currentTime = now();
      const retentionMs = scope.retentionSeconds * 1_000;
      prepare(currentTime);

      let entries = loadEntries(options.storage);
      for (const entry of entries) {
        const expired =
          entry.expiresAt <= currentTime ||
          (entry.endpoint === endpoint &&
            entry.namespace === scope.namespace &&
            entry.createdAt + retentionMs <= currentTime);
        const replacedStore = entry.endpoint === endpoint && entry.namespace !== scope.namespace;
        if (expired || replacedStore) removeEntry(entry);
      }
      entries = loadEntries(options.storage);

      const owners = loadOwnerLeases(options.storage, currentTime);
      const activeOwners = new Set(owners.map((owner) => owner.owner));
      const leader = owners.toSorted(
        (left, right) => left.startedAt - right.startedAt || left.owner.localeCompare(right.owner),
      )[0]?.owner;
      const canonical = `${method}\u0000${canonicalJSON(params)}`;
      const matching = entries
        .filter(
          (entry) =>
            entry.endpoint === endpoint &&
            entry.namespace === scope.namespace &&
            entry.fingerprint === fingerprint(`${entry.salt}\u0000${canonical}`),
        )
        .toSorted((left, right) => left.createdAt - right.createdAt);

      const preferredEntry =
        preferredKey === undefined
          ? undefined
          : entries.find((candidate) => candidate.idempotencyKey === preferredKey);
      if (preferredEntry && !matching.includes(preferredEntry)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity candidate belongs to a different command",
        );
      }
      if (
        preferredEntry?.owner === PROCESS_OWNER &&
        activeClaims.get(preferredEntry.idempotencyKey) !== undefined &&
        activeClaims.get(preferredEntry.idempotencyKey) !== journalInstance
      ) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity is owned by a successor Desktop client",
        );
      }

      let entry = matching.find(
        (candidate) =>
          candidate.idempotencyKey === preferredKey && candidate.owner === PROCESS_OWNER,
      );
      entry ??= matching.find(
        (candidate) =>
          candidate.owner === PROCESS_OWNER &&
          (candidate.claimable || PROCESS_CLAIMABLE.has(candidate.idempotencyKey)),
      );
      if (!entry) {
        entry = matching.find(
          (candidate) => !activeOwners.has(candidate.owner) && leader === PROCESS_OWNER,
        );
      }
      if (!entry) {
        const uncertainOwner = matching.find(
          (candidate) =>
            candidate.owner !== PROCESS_OWNER &&
            (activeOwners.has(candidate.owner) || leader !== PROCESS_OWNER),
        );
        if (uncertainOwner) {
          throw new MutationJournalOwnershipError(
            "A matching Runtime mutation is still owned by another live Desktop window",
          );
        }
      }

      let createdFresh = false;
      if (entry) {
        entry = { ...entry, owner: PROCESS_OWNER, claimable: false };
      } else {
        if (entries.length >= MAX_ENTRIES) {
          throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
        }
        const salt = crypto.randomUUID();
        entry = {
          version: JOURNAL_VERSION,
          salt,
          endpoint,
          namespace: scope.namespace,
          fingerprint: fingerprint(`${salt}\u0000${canonical}`),
          idempotencyKey: preferredKey ?? crypto.randomUUID(),
          owner: PROCESS_OWNER,
          claimable: false,
          createdAt: currentTime,
          expiresAt: currentTime + retentionMs,
        };
        createdFresh = true;
      }
      try {
        persistEntry(entry);
      } catch (error) {
        // No reservation was returned, so this newly-created command cannot
        // have reached the transport. Remember it only to preserve its exact
        // key for retry and to remove a write-that-landed during disposal.
        if (createdFresh) unconfirmedUnsent.add(entry.idempotencyKey);
        throw error;
      }
      unconfirmedUnsent.delete(entry.idempotencyKey);
      activeClaims.set(entry.idempotencyKey, journalInstance);
      PROCESS_CLAIMABLE.delete(entry.idempotencyKey);
      startHeartbeat();

      const lifecycle = lifecycleFor(entry);
      return {
        idempotencyKey: entry.idempotencyKey,
        track: (mutation) => trackedMutation(mutation, lifecycle),
      };
    },
    dispose() {
      if (disposed) return;
      if (heartbeat !== undefined) clearInterval(heartbeat);
      heartbeat = undefined;
      let failure: unknown;
      for (const idempotencyKey of unconfirmedUnsent) {
        try {
          removeValue(options.storage, entryKey(idempotencyKey));
          unconfirmedUnsent.delete(idempotencyKey);
          PROCESS_CLAIMABLE.delete(idempotencyKey);
        } catch (error) {
          failure ??= error;
        }
      }
      for (const [idempotencyKey, owner] of activeClaims) {
        if (owner !== journalInstance) continue;
        try {
          const current = readEntry(idempotencyKey);
          if (current?.owner === PROCESS_OWNER && !current.claimable) {
            persistEntry({ ...current, claimable: true });
          }
          PROCESS_CLAIMABLE.delete(idempotencyKey);
        } catch (error) {
          // Persistence may have failed after the write reached the host. Keep
          // the identity recoverable by a replacement adapter in this process;
          // a real process restart falls back to the durable owner lease.
          PROCESS_CLAIMABLE.add(idempotencyKey);
          failure ??= error;
        } finally {
          if (activeClaims.get(idempotencyKey) === journalInstance) {
            activeClaims.delete(idempotencyKey);
          }
        }
      }
      if (activeClaims.size === 0) {
        try {
          removeValue(options.storage, ownerKey(PROCESS_OWNER));
        } catch (error) {
          failure ??= error;
        }
      }
      disposed = true;
      if (failure !== undefined) throw failure;
    },
  };
}
