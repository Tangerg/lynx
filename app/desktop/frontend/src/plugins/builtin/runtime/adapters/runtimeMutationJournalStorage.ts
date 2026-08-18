import type { KeyValueStore } from "@/plugins/sdk";
import { configureRuntimeMutationJournalStorage } from "../application/ports/mutationJournal";

const STORAGE_PREFIX = "mutation-journal-v3.";
const PROBE_STORAGE_PREFIX = `${STORAGE_PREFIX}probe:`;

function storageKey(key: string): string {
  return `${STORAGE_PREFIX}${key}`;
}

function storedValuesEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

/** Bind the RPC mutation journal to this Runtime context's namespaced Host
 * storage. The adapter never interprets protocol methods, params, or keys. */
export function installRuntimeMutationJournalStorage(ctx: { storage: KeyValueStore }): () => void {
  return configureRuntimeMutationJournalStorage({
    get: (key) => ctx.storage.get(storageKey(key)),
    set: (key, value) => {
      if (value === undefined) {
        throw new Error(`Runtime mutation journal cannot persist undefined: ${key}`);
      }
      const target = storageKey(key);
      ctx.storage.set(target, value);
      if (!storedValuesEqual(ctx.storage.get(target), value)) {
        throw new Error(`Runtime mutation journal write was not persisted: ${key}`);
      }
    },
    remove: (key) => {
      const target = storageKey(key);
      ctx.storage.remove(target);
      if (ctx.storage.get(target) !== undefined) {
        throw new Error(`Runtime mutation journal removal was not persisted: ${key}`);
      }
    },
    keys: () => {
      const probeStorageKey = `${PROBE_STORAGE_PREFIX}${crypto.randomUUID()}`;
      const probe = crypto.randomUUID();
      ctx.storage.set(probeStorageKey, probe);
      if (ctx.storage.get(probeStorageKey) !== probe) {
        ctx.storage.remove(probeStorageKey);
        if (ctx.storage.get(probeStorageKey) !== undefined) {
          throw new Error("Runtime mutation journal failed probe was not removed");
        }
        throw new Error("Runtime mutation journal storage is not writable");
      }
      let keys: string[] = [];
      let enumerationError: unknown;
      try {
        keys = ctx.storage.keys();
        if (!keys.includes(probeStorageKey)) {
          throw new Error("Runtime mutation journal storage enumeration is incomplete");
        }
      } catch (error) {
        enumerationError = error;
      }
      ctx.storage.remove(probeStorageKey);
      if (ctx.storage.get(probeStorageKey) !== undefined) {
        throw new Error("Runtime mutation journal storage probe was not removed");
      }
      if (enumerationError !== undefined) throw enumerationError;
      return keys
        .filter((key) => key.startsWith(STORAGE_PREFIX) && !key.startsWith(PROBE_STORAGE_PREFIX))
        .map((key) => key.slice(STORAGE_PREFIX.length));
    },
  });
}
