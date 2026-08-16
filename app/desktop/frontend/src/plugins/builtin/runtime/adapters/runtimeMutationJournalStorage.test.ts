import { afterEach, describe, expect, it, vi } from "vitest";
import { getContainer, resetContainer } from "@/main/container";
import type { KeyValueStore } from "@/plugins/sdk";
import { installedRuntimeMutationJournalStorage } from "../application/ports/mutationJournal";
import { installRuntimeMutationJournalStorage } from "./runtimeMutationJournalStorage";

const cleanups: Array<() => void> = [];

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup();
  await resetContainer();
});

describe("Runtime mutation journal storage adapter", () => {
  it("owns verified opaque records in Runtime plugin storage and disconnects on unload", () => {
    const stored = new Map<string, unknown>();
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    };
    const dispose = installRuntimeMutationJournalStorage(ctx);
    cleanups.push(dispose);

    const storage = installedRuntimeMutationJournalStorage();
    expect(storage).not.toBeNull();
    stored.set("mutation-journal-v2.probe:other-window", "stale-probe");
    storage?.set("entry:key-1", { version: 2 });
    storage?.set("owner:window-1", { version: 1 });
    expect(storage?.get("entry:key-1")).toEqual({ version: 2 });
    expect(storage?.keys().toSorted()).toEqual(["entry:key-1", "owner:window-1"]);
    expect(stored.has("mutation-journal-v2.entry:key-1")).toBe(true);
    storage?.remove("entry:key-1");
    expect(storage?.get("entry:key-1")).toBeUndefined();

    dispose();
    expect(installedRuntimeMutationJournalStorage()).toBeNull();
  });

  it("maps the shipped v1 snapshot only through the explicit migration key", () => {
    const stored = new Map<string, unknown>([["mutation-journal-v1", { version: 1 }]]);
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    };
    cleanups.push(installRuntimeMutationJournalStorage(ctx));
    const storage = installedRuntimeMutationJournalStorage();

    expect(storage?.get("legacy:v1")).toEqual({ version: 1 });
    expect(storage?.keys()).toEqual([]);
    storage?.remove("legacy:v1");
    expect(stored.has("mutation-journal-v1")).toBe(false);
  });

  it("surfaces Host storage writes and removals that were swallowed", () => {
    const stored = new Map<string, unknown>();
    let discardWrites = true;
    let discardRemovals = false;
    let hideKeys = false;
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => {
          if (!discardWrites) stored.set(key, value);
        },
        remove: (key) => {
          if (!discardRemovals) stored.delete(key);
        },
        keys: () => (hideKeys ? [] : [...stored.keys()]),
        clear: () => stored.clear(),
      },
    };
    cleanups.push(installRuntimeMutationJournalStorage(ctx));
    const storage = installedRuntimeMutationJournalStorage()!;

    expect(() => storage.set("entry:key-1", { version: 2 })).toThrow("not persisted");
    discardWrites = false;
    storage.set("entry:key-1", { version: 2 });
    hideKeys = true;
    expect(() => storage.keys()).toThrow("enumeration is incomplete");
    hideKeys = false;
    discardRemovals = true;
    expect(() => storage.remove("entry:key-1")).toThrow("not persisted");
  });

  it("rebuilds a client that was constructed before journal storage installed", () => {
    const stored = new Map<string, unknown>();
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    };
    const beforeInstall = getContainer().client();
    const closeBeforeInstall = vi.spyOn(beforeInstall, "close");

    cleanups.push(installRuntimeMutationJournalStorage(ctx));
    const afterInstall = getContainer().client();

    expect(afterInstall).not.toBe(beforeInstall);
    expect(closeBeforeInstall).toHaveBeenCalledOnce();
    expect(getContainer().client()).toBe(afterInstall);
  });
});
