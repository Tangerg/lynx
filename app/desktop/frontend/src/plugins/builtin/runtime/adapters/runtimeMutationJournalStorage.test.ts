import { afterEach, describe, expect, it } from "vitest";
import { getContainer, resetContainer } from "@/main/container";
import type { Host } from "@/plugins/sdk";
import { installedRuntimeMutationJournalStorage } from "../application/ports/mutationJournal";
import { installRuntimeMutationJournalStorage } from "./runtimeMutationJournalStorage";

const cleanups: Array<() => void> = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup();
  resetContainer();
});

describe("Runtime mutation journal storage adapter", () => {
  it("owns one opaque snapshot in Runtime plugin storage and disconnects on unload", () => {
    const stored = new Map<string, unknown>();
    const host: Pick<Host, "storage"> = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
      },
    };
    const dispose = installRuntimeMutationJournalStorage(host);
    cleanups.push(dispose);

    const storage = installedRuntimeMutationJournalStorage();
    expect(storage).not.toBeNull();
    storage?.write({ version: 1, entries: [] });
    expect(storage?.read()).toEqual({ version: 1, entries: [] });

    dispose();
    expect(installedRuntimeMutationJournalStorage()).toBeNull();
  });

  it("rebuilds a client that was constructed before journal storage installed", () => {
    const stored = new Map<string, unknown>();
    const host: Pick<Host, "storage"> = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
      },
    };
    const beforeInstall = getContainer().client();

    cleanups.push(installRuntimeMutationJournalStorage(host));
    const afterInstall = getContainer().client();

    expect(afterInstall).not.toBe(beforeInstall);
    expect(getContainer().client()).toBe(afterInstall);
  });
});
