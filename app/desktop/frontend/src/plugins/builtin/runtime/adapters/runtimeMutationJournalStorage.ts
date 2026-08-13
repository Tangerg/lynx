import type { Host } from "@/plugins/sdk";
import { configureRuntimeMutationJournalStorage } from "../application/ports/mutationJournal";

const STORAGE_PREFIX = "mutation-journal-v2.";
const LEGACY_STORAGE_KEY = "mutation-journal-v1";
const LEGACY_JOURNAL_KEY = "legacy:v1";
const PROBE_STORAGE_PREFIX = `${STORAGE_PREFIX}probe:`;

function storageKey(key: string): string {
  return `${STORAGE_PREFIX}${key}`;
}

function storedValuesEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

/** Bind the RPC mutation journal to this Runtime context's namespaced Host
 * storage. The adapter never interprets protocol methods, params, or keys. */
export function installRuntimeMutationJournalStorage(host: Pick<Host, "storage">): () => void {
  return configureRuntimeMutationJournalStorage({
    get: (key) =>
      host.storage.get(key === LEGACY_JOURNAL_KEY ? LEGACY_STORAGE_KEY : storageKey(key)),
    set: (key, value) => {
      if (value === undefined) {
        throw new Error(`Runtime mutation journal cannot persist undefined: ${key}`);
      }
      const target = storageKey(key);
      host.storage.set(target, value);
      if (!storedValuesEqual(host.storage.get(target), value)) {
        throw new Error(`Runtime mutation journal write was not persisted: ${key}`);
      }
    },
    remove: (key) => {
      const target = key === LEGACY_JOURNAL_KEY ? LEGACY_STORAGE_KEY : storageKey(key);
      host.storage.remove(target);
      if (host.storage.get(target) !== undefined) {
        throw new Error(`Runtime mutation journal removal was not persisted: ${key}`);
      }
    },
    keys: () => {
      const probeStorageKey = `${PROBE_STORAGE_PREFIX}${crypto.randomUUID()}`;
      const probe = crypto.randomUUID();
      host.storage.set(probeStorageKey, probe);
      if (host.storage.get(probeStorageKey) !== probe) {
        host.storage.remove(probeStorageKey);
        if (host.storage.get(probeStorageKey) !== undefined) {
          throw new Error("Runtime mutation journal failed probe was not removed");
        }
        throw new Error("Runtime mutation journal storage is not writable");
      }
      let keys: string[] = [];
      let enumerationError: unknown;
      try {
        keys = host.storage.keys();
        if (!keys.includes(probeStorageKey)) {
          throw new Error("Runtime mutation journal storage enumeration is incomplete");
        }
      } catch (error) {
        enumerationError = error;
      }
      host.storage.remove(probeStorageKey);
      if (host.storage.get(probeStorageKey) !== undefined) {
        throw new Error("Runtime mutation journal storage probe was not removed");
      }
      if (enumerationError !== undefined) throw enumerationError;
      return keys
        .filter((key) => key.startsWith(STORAGE_PREFIX) && !key.startsWith(PROBE_STORAGE_PREFIX))
        .map((key) => key.slice(STORAGE_PREFIX.length));
    },
  });
}
