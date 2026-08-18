const JOURNAL_VERSION = 2;
const MAX_ENTRIES = 256;
const ENTRY_PREFIX = "entry:";

export interface MutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}

export interface MutationJournalScope {
  /** Opaque identity published by the Runtime's durable idempotency store. */
  namespace: string;
  retentionSeconds: number;
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

export class MutationJournalScopeUnavailableError extends MutationJournalError {
  override readonly name = "MutationJournalScopeUnavailableError";
}

export class MutationJournalCapacityError extends MutationJournalError {
  override readonly name = "MutationJournalCapacityError";
}

interface JournalEntry {
  version: typeof JOURNAL_VERSION;
  salt: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  createdAt: number;
  expiresAt: number;
}

export interface DurableMutationIdentity {
  readonly entry: JournalEntry;
}

export interface DurableMutationJournal {
  reserve(
    method: string,
    params: unknown,
    preferredKey: string | undefined,
    claimed: (idempotencyKey: string) => boolean,
  ): DurableMutationIdentity | undefined;
  authorize(identity: DurableMutationIdentity): string;
  retain(identity: DurableMutationIdentity): void;
  settle(identity: DurableMutationIdentity): void;
}

interface DurableMutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now: () => number;
}

function storageError(message: string, cause?: unknown): MutationJournalStorageError {
  if (cause instanceof MutationJournalStorageError) return cause;
  return new MutationJournalStorageError(message, cause === undefined ? undefined : { cause });
}

function validText(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= maximum;
}

function validEntry(value: unknown): value is JournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<JournalEntry>;
  return (
    Object.keys(value).length === 7 &&
    entry.version === JOURNAL_VERSION &&
    validText(entry.salt, 255) &&
    validText(entry.namespace, 255) &&
    typeof entry.fingerprint === "string" &&
    /^[0-9a-f]{32}$/.test(entry.fingerprint) &&
    validText(entry.idempotencyKey, 255) &&
    typeof entry.createdAt === "number" &&
    Number.isFinite(entry.createdAt) &&
    typeof entry.expiresAt === "number" &&
    Number.isFinite(entry.expiresAt) &&
    entry.expiresAt > entry.createdAt
  );
}

function entryKey(idempotencyKey: string): string {
  return `${ENTRY_PREFIX}${encodeURIComponent(idempotencyKey)}`;
}

function validScope(scope: MutationJournalScope | null | undefined): scope is MutationJournalScope {
  if (!scope) return false;
  return (
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
    throw storageError("Runtime mutation journal keys are unavailable", error);
  }
}

function readValue(storage: MutationJournalStorage, key: string): unknown {
  try {
    return storage.get(key);
  } catch (error) {
    throw storageError(`Runtime mutation journal record is unreadable: ${key}`, error);
  }
}

function persistValue(storage: MutationJournalStorage, key: string, value: unknown): void {
  try {
    storage.set(key, value);
  } catch (error) {
    throw storageError(`Runtime mutation journal record was not persisted: ${key}`, error);
  }
}

function removeValue(storage: MutationJournalStorage, key: string): void {
  try {
    storage.remove(key);
  } catch (error) {
    throw storageError(`Runtime mutation journal record was not removed: ${key}`, error);
  }
}

function loadEntries(storage: MutationJournalStorage): JournalEntry[] {
  return readKeys(storage)
    .filter((key) => key.startsWith(ENTRY_PREFIX))
    .map((key) => {
      const value = readValue(storage, key);
      if (!validEntry(value) || entryKey(value.idempotencyKey) !== key) {
        throw storageError(`Runtime mutation journal entry is corrupted: ${key}`);
      }
      return value;
    });
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

/** A compact 128-bit matching fingerprint. Runtime still validates the real
 * method and params; this value is only a privacy-preserving local lookup key. */
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

function sameEntry(left: JournalEntry, right: JournalEntry): boolean {
  return (
    left.version === right.version &&
    left.salt === right.salt &&
    left.namespace === right.namespace &&
    left.fingerprint === right.fingerprint &&
    left.idempotencyKey === right.idempotencyKey &&
    left.createdAt === right.createdAt &&
    left.expiresAt === right.expiresAt
  );
}

export function openDurableMutationJournal(
  options: DurableMutationJournalOptions,
): DurableMutationJournal {
  const readEntry = (idempotencyKey: string): JournalEntry | undefined => {
    const key = entryKey(idempotencyKey);
    const value = readValue(options.storage, key);
    if (value === undefined) return undefined;
    if (!validEntry(value) || entryKey(value.idempotencyKey) !== key) {
      throw storageError(`Runtime mutation journal entry is corrupted: ${key}`);
    }
    return value;
  };

  const persistEntry = (entry: JournalEntry) =>
    persistValue(options.storage, entryKey(entry.idempotencyKey), entry);

  const removeEntry = (entry: JournalEntry) =>
    removeValue(options.storage, entryKey(entry.idempotencyKey));

  const currentScope = () => {
    const scope = options.scope();
    if (!validScope(scope)) {
      throw new MutationJournalScopeUnavailableError(
        "Runtime mutation store identity is temporarily unavailable",
      );
    }
    return scope;
  };

  const validateIdentity = (identity: DurableMutationIdentity, scope: MutationJournalScope) => {
    const entry = identity.entry;
    if (entry.namespace !== scope.namespace) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity belongs to a replaced Runtime store",
      );
    }
    const effectiveExpiry = Math.min(
      entry.expiresAt,
      entry.createdAt + scope.retentionSeconds * 1_000,
    );
    if (effectiveExpiry <= options.now()) {
      throw new MutationJournalError("Runtime mutation identity expired before delivery");
    }
  };

  return {
    reserve(method, params, preferredKey, claimed) {
      const scope = options.scope();
      if (!validScope(scope)) return undefined;
      if (preferredKey !== undefined && !validText(preferredKey, 255)) {
        throw new MutationJournalError("Runtime mutation identity candidate is invalid");
      }
      const currentTime = options.now();
      const retentionMs = scope.retentionSeconds * 1_000;
      let entries = loadEntries(options.storage);
      for (const entry of entries) {
        if (entry.expiresAt <= currentTime || entry.namespace !== scope.namespace) {
          removeEntry(entry);
        }
      }
      entries = loadEntries(options.storage);
      const canonicalCommand = `${method}\u0000${canonicalJSON(params)}`;
      const matches = (entry: JournalEntry) =>
        entry.namespace === scope.namespace &&
        entry.fingerprint === fingerprint(`${entry.salt}\u0000${canonicalCommand}`);
      const preferred =
        preferredKey === undefined
          ? undefined
          : entries.find((entry) => entry.idempotencyKey === preferredKey);
      if (preferred && !matches(preferred)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity candidate belongs to a different command",
        );
      }
      if (preferred && claimed(preferred.idempotencyKey)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity is already owned by an active command",
        );
      }
      let entry = preferred;
      entry ??= entries
        .filter((candidate) => matches(candidate) && !claimed(candidate.idempotencyKey))
        .toSorted((left, right) => left.createdAt - right.createdAt)[0];
      if (!entry) {
        if (entries.length >= MAX_ENTRIES) {
          throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
        }
        const salt = crypto.randomUUID();
        entry = {
          version: JOURNAL_VERSION,
          salt,
          namespace: scope.namespace,
          fingerprint: fingerprint(`${salt}\u0000${canonicalCommand}`),
          idempotencyKey: preferredKey ?? crypto.randomUUID(),
          createdAt: currentTime,
          expiresAt: currentTime + retentionMs,
        };
        persistEntry(entry);
      }
      return { entry };
    },
    authorize(identity) {
      const scope = currentScope();
      validateIdentity(identity, scope);
      const persisted = readEntry(identity.entry.idempotencyKey);
      if (persisted && !sameEntry(persisted, identity.entry)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity no longer names the reserved command",
        );
      }
      return identity.entry.namespace;
    },
    retain(identity) {
      const scope = currentScope();
      validateIdentity(identity, scope);
      const persisted = readEntry(identity.entry.idempotencyKey);
      if (persisted) {
        if (!sameEntry(persisted, identity.entry)) {
          throw new MutationJournalOwnershipError(
            "Runtime mutation identity no longer names the reserved command",
          );
        }
        return;
      }
      persistEntry(identity.entry);
    },
    settle(identity) {
      const persisted = readEntry(identity.entry.idempotencyKey);
      if (!persisted) return;
      if (!sameEntry(persisted, identity.entry)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity no longer names the reserved command",
        );
      }
      removeEntry(persisted);
    },
  };
}
