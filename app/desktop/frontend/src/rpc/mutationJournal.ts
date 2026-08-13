import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

const JOURNAL_VERSION = 1;
const MAX_ENTRIES = 256;
const PROCESS_OWNER = crypto.randomUUID();

export interface MutationJournalStorage {
  read(): unknown;
  write(snapshot: unknown): void;
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
  reserve(method: string, params: unknown): MutationReservation | undefined;
}

export interface MutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now?: () => number;
}

interface JournalEntry {
  endpoint: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  owner: string;
  claimable: boolean;
  createdAt: number;
  expiresAt: number;
}

interface JournalSnapshot {
  version: typeof JOURNAL_VERSION;
  salt: string;
  entries: JournalEntry[];
}

function validEntry(value: unknown): value is JournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<JournalEntry>;
  return (
    typeof entry.endpoint === "string" &&
    entry.endpoint.length > 0 &&
    entry.endpoint.length <= 2_048 &&
    typeof entry.namespace === "string" &&
    entry.namespace.length > 0 &&
    entry.namespace.length <= 255 &&
    typeof entry.fingerprint === "string" &&
    /^[0-9a-f]{32}$/.test(entry.fingerprint) &&
    typeof entry.idempotencyKey === "string" &&
    entry.idempotencyKey.length > 0 &&
    entry.idempotencyKey.length <= 255 &&
    typeof entry.owner === "string" &&
    entry.owner.length > 0 &&
    entry.owner.length <= 255 &&
    typeof entry.claimable === "boolean" &&
    typeof entry.createdAt === "number" &&
    Number.isFinite(entry.createdAt) &&
    typeof entry.expiresAt === "number" &&
    Number.isFinite(entry.expiresAt) &&
    entry.expiresAt > entry.createdAt
  );
}

function readSnapshot(storage: MutationJournalStorage): JournalSnapshot {
  const value = storage.read();
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const snapshot = value as Partial<JournalSnapshot>;
    if (
      snapshot.version === JOURNAL_VERSION &&
      typeof snapshot.salt === "string" &&
      snapshot.salt.length > 0 &&
      snapshot.salt.length <= 255 &&
      Array.isArray(snapshot.entries)
    ) {
      return {
        version: JOURNAL_VERSION,
        salt: snapshot.salt,
        entries: snapshot.entries.slice(-MAX_ENTRIES).filter(validEntry),
      };
    }
  }
  return { version: JOURNAL_VERSION, salt: crypto.randomUUID(), entries: [] };
}

function normalizeEndpoint(endpoint: string): string {
  return endpoint.replace(/\/+$/, "");
}

function validScope(scope: MutationJournalScope | null | undefined): scope is MutationJournalScope {
  return (
    !!scope &&
    normalizeEndpoint(scope.endpoint).length > 0 &&
    scope.namespace.length > 0 &&
    Number.isInteger(scope.retentionSeconds) &&
    scope.retentionSeconds > 0
  );
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

function trackedMutation<T>(
  mutation: MutationPromise<T>,
  claim: () => void,
  resolve: () => void,
  reject: (error: unknown) => void,
): MutationPromise<T> {
  const tracked = mutation.then(
    (value) => {
      resolve();
      return value;
    },
    (error: unknown) => {
      reject(error);
      throw error;
    },
  );
  return Object.defineProperties(tracked, {
    idempotencyKey: { enumerable: true, get: () => mutation.idempotencyKey },
    retry: {
      enumerable: true,
      value: (options?: { signal?: AbortSignal }) => {
        claim();
        return trackedMutation(mutation.retry(options), claim, resolve, reject);
      },
    },
  }) as MutationPromise<T>;
}

/**
 * Retain unresolved mutation identities across Desktop process restarts.
 *
 * Entries are written before the first transport attempt. At construction, old
 * entries become replay candidates; entries opened in this process stay claimed
 * so concurrent same-shaped product intents still receive distinct keys. Only an
 * ambiguous outcome makes an entry claimable again. Endpoint and Runtime-owned
 * namespace must both match before a key can be restored.
 */
export function createMutationJournal(options: MutationJournalOptions): MutationJournal {
  const snapshot = readSnapshot(options.storage);
  const now = options.now ?? Date.now;
  let entries = snapshot.entries;
  const available = new Set(
    entries
      .filter((entry) => entry.owner !== PROCESS_OWNER || entry.claimable)
      .map((entry) => entry.idempotencyKey),
  );

  const save = () => {
    options.storage.write({ ...snapshot, entries });
  };

  const remove = (entry: JournalEntry) => {
    entries = entries.filter((candidate) => candidate !== entry);
    available.delete(entry.idempotencyKey);
    save();
  };

  return {
    reserve(method, params) {
      const scope = options.scope();
      if (!validScope(scope)) return undefined;
      const endpoint = normalizeEndpoint(scope.endpoint);
      const currentTime = now();
      const retentionMs = scope.retentionSeconds * 1_000;

      entries = entries.filter((entry) => {
        const expired =
          entry.expiresAt <= currentTime ||
          (entry.endpoint === endpoint &&
            entry.namespace === scope.namespace &&
            entry.createdAt + retentionMs <= currentTime);
        const replacedStore = entry.endpoint === endpoint && entry.namespace !== scope.namespace;
        if (expired || replacedStore) available.delete(entry.idempotencyKey);
        return !expired && !replacedStore;
      });

      const keyFingerprint = fingerprint(
        `${snapshot.salt}\u0000${method}\u0000${canonicalJSON(params)}`,
      );
      let entry = entries.find(
        (candidate) =>
          candidate.endpoint === endpoint &&
          candidate.namespace === scope.namespace &&
          candidate.fingerprint === keyFingerprint &&
          available.has(candidate.idempotencyKey),
      );
      if (entry) {
        available.delete(entry.idempotencyKey);
        entry.owner = PROCESS_OWNER;
        entry.claimable = false;
      } else {
        entry = {
          endpoint,
          namespace: scope.namespace,
          fingerprint: keyFingerprint,
          idempotencyKey: crypto.randomUUID(),
          owner: PROCESS_OWNER,
          claimable: false,
          createdAt: currentTime,
          expiresAt: currentTime + retentionMs,
        };
        entries.push(entry);
        if (entries.length > MAX_ENTRIES) {
          entries = entries
            .sort((left, right) => left.createdAt - right.createdAt)
            .slice(-MAX_ENTRIES);
        }
      }
      // Persist before any transport work begins. A process loss immediately
      // after reserve() must leave enough identity to replay safely.
      save();

      const claim = () => {
        available.delete(entry.idempotencyKey);
        entry.owner = PROCESS_OWNER;
        entry.claimable = false;
        if (!entries.includes(entry)) entries.push(entry);
        save();
      };
      const resolve = () => remove(entry);
      const reject = (error: unknown) => {
        if (mutationSettlementIsUnknown(error)) {
          entry.claimable = true;
          available.add(entry.idempotencyKey);
          save();
          return;
        }
        remove(entry);
      };
      return {
        idempotencyKey: entry.idempotencyKey,
        track: (mutation) => trackedMutation(mutation, claim, resolve, reject),
      };
    },
  };
}
