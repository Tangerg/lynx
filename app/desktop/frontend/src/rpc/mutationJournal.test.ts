import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "./errors";
import { createMutationPromise } from "./mutation";
import {
  createMutationJournal,
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalStorageError,
  type MutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
} from "./mutationJournal";

function clone<T>(value: T): T {
  return value === undefined ? value : structuredClone(value);
}

class MemoryStorage implements MutationJournalStorage {
  readonly values = new Map<string, unknown>();
  failGet = false;
  failSet = false;
  failRemove = false;
  failSetKey: string | undefined;

  constructor(legacy?: unknown) {
    if (legacy !== undefined) this.values.set("legacy:v1", clone(legacy));
  }

  get(key: string): unknown {
    if (this.failGet) throw new Error("read failed");
    return clone(this.values.get(key));
  }

  set(key: string, value: unknown): void {
    if (this.failSet || key === this.failSetKey) throw new Error("write failed");
    this.values.set(key, clone(value));
  }

  remove(key: string): void {
    if (this.failRemove) throw new Error("remove failed");
    this.values.delete(key);
  }

  keys(): string[] {
    return [...this.values.keys()];
  }

  copy(): MemoryStorage {
    const copied = new MemoryStorage();
    for (const [key, value] of this.values) copied.values.set(key, clone(value));
    return copied;
  }

  entryValues(): Array<Record<string, unknown>> {
    const current = new Map<string, Record<string, unknown>>();
    for (const [, value] of [...this.values.entries()]
      .filter(([key]) => key.startsWith("entry:"))
      .map(([key, value]) => [key, value as Record<string, unknown>] as const)) {
      const idempotencyKey = String(value.idempotencyKey);
      const existing = current.get(idempotencyKey);
      const generation = Number(value.generation ?? 0);
      const existingGeneration = Number(existing?.generation ?? 0);
      const generationId = String(value.generationId ?? "");
      const existingGenerationId = String(existing?.generationId ?? "");
      if (
        !existing ||
        generation > existingGeneration ||
        (generation === existingGeneration && generationId > existingGenerationId)
      ) {
        current.set(idempotencyKey, value);
      }
    }
    return [...current.values()].filter((entry) => entry.settled !== true);
  }
}

const scope: MutationJournalScope = {
  endpoint: "http://127.0.0.1:17171/",
  namespace: "idp_store_a",
  retentionSeconds: 86_400,
};

const journals: MutationJournal[] = [];

function journal(options: Parameters<typeof createMutationJournal>[0]): MutationJournal {
  const created = createMutationJournal(options);
  journals.push(created);
  return created;
}

afterEach(() => {
  for (const created of journals.splice(0)) created.dispose();
  vi.useRealTimers();
});

function opening(
  reservation: NonNullable<ReturnType<MutationJournal["reserve"]>>,
  execute: () => Promise<string>,
) {
  return reservation.track(createMutationPromise(() => execute(), reservation.idempotencyKey));
}

function persistedEntry(storage: MemoryStorage, idempotencyKey: string): Record<string, unknown> {
  const entry = storage.get(persistedEntryKey(storage, idempotencyKey));
  expect(entry).toBeDefined();
  return entry as Record<string, unknown>;
}

function persistedEntryKey(storage: MemoryStorage, idempotencyKey: string): string {
  const candidates = [...storage.values.entries()]
    .filter(
      ([key, value]) =>
        key.startsWith("entry:") &&
        (value as Record<string, unknown> | undefined)?.idempotencyKey === idempotencyKey,
    )
    .toSorted(([, left], [, right]) => {
      const leftEntry = left as Record<string, unknown>;
      const rightEntry = right as Record<string, unknown>;
      const generation = Number(rightEntry.generation ?? 0) - Number(leftEntry.generation ?? 0);
      if (generation !== 0) return generation;
      const leftId = String(leftEntry.generationId ?? "");
      const rightId = String(rightEntry.generationId ?? "");
      if (leftId === rightId) return 0;
      return leftId < rightId ? 1 : -1;
    });
  expect(candidates[0]).toBeDefined();
  return candidates[0]![0];
}

describe("persistent mutation journal", () => {
  it("persists a privacy-preserving identity before opening the transport", () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const reservation = created.reserve("providers.update", {
      provider: "secret-provider",
      apiKey: "must-not-enter-storage",
    });

    expect(reservation?.idempotencyKey).toBeTruthy();
    const encoded = JSON.stringify([...storage.values]);
    expect(encoded).not.toContain("secret-provider");
    expect(encoded).not.toContain("must-not-enter-storage");
  });

  it("keeps independent entries from separate journal instances without lost updates", () => {
    const storage = new MemoryStorage();
    journal({ storage, scope: () => scope }).reserve("sessions.delete", { sessionId: "ses_1" });
    journal({ storage, scope: () => scope }).reserve("sessions.delete", { sessionId: "ses_2" });

    expect(storage.entryValues()).toHaveLength(2);
  });

  it("restores one crashed command without merging a live same-process twin", () => {
    const storage = new MemoryStorage();
    const firstJournal = journal({ storage, scope: () => scope });
    const original = firstJournal.reserve("schedules.runNow", { id: "schedule_1" })!;

    const crashImage = storage.copy();
    const sameProcess = journal({ storage, scope: () => scope });
    expect(sameProcess.reserve("schedules.runNow", { id: "schedule_1" })?.idempotencyKey).not.toBe(
      original.idempotencyKey,
    );

    const persisted = persistedEntry(crashImage, original.idempotencyKey);
    persisted.owner = "previous-process";
    crashImage.set(persistedEntryKey(crashImage, original.idempotencyKey), persisted);

    const restarted = journal({ storage: crashImage, scope: () => scope });
    const replay = restarted.reserve("schedules.runNow", { id: "schedule_1" })!;
    const freshTwin = restarted.reserve("schedules.runNow", { id: "schedule_1" })!;

    expect(replay.idempotencyKey).toBe(original.idempotencyKey);
    expect(freshTwin.idempotencyKey).not.toBe(original.idempotencyKey);
  });

  it("blocks a duplicate while another window lease may still be alive, then restores it", () => {
    const storage = new MemoryStorage();
    let currentTime = 1_000;
    const first = journal({ storage, scope: () => scope, now: () => currentTime }).reserve(
      "goals.resume",
      { sessionId: "ses_1" },
    )!;
    const persisted = persistedEntry(storage, first.idempotencyKey);
    persisted.owner = "other-window";
    storage.set(persistedEntryKey(storage, first.idempotencyKey), persisted);
    storage.set("owner:other-window", {
      version: 1,
      owner: "other-window",
      startedAt: 0,
      expiresAt: 31_000,
    });

    const secondWindow = journal({ storage, scope: () => scope, now: () => currentTime });
    expect(() => secondWindow.reserve("goals.resume", { sessionId: "ses_1" })).toThrow(
      MutationJournalOwnershipError,
    );
    expect(storage.entryValues()).toHaveLength(1);

    currentTime = 31_001;
    expect(secondWindow.reserve("goals.resume", { sessionId: "ses_1" })?.idempotencyKey).toBe(
      first.idempotencyKey,
    );
  });

  it("does not delete a lease renewed by another renderer after the expiry read", () => {
    const backing = new MemoryStorage();
    let currentTime = 31_001;
    const original = journal({ storage: backing, scope: () => scope, now: () => 1_000 }).reserve(
      "goals.resume",
      { sessionId: "ses_renewed" },
    )!;
    const persisted = persistedEntry(backing, original.idempotencyKey);
    persisted.owner = "renewing-renderer";
    backing.set(persistedEntryKey(backing, original.idempotencyKey), persisted);
    const expiredLeaseKey = "owner:renewing-renderer:expired-generation";
    const renewedLeaseKey = "owner:renewing-renderer:renewed-generation";
    backing.set(expiredLeaseKey, {
      version: 2,
      owner: "renewing-renderer",
      leaseId: "expired-generation",
      startedAt: 0,
      expiresAt: 31_000,
    });
    let injectedRenewal = false;
    const interleaved: MutationJournalStorage = {
      get(key) {
        const value = backing.get(key);
        if (key === expiredLeaseKey && !injectedRenewal) {
          injectedRenewal = true;
          backing.set(renewedLeaseKey, {
            version: 2,
            owner: "renewing-renderer",
            leaseId: "renewed-generation",
            startedAt: 0,
            expiresAt: 61_001,
          });
        }
        return value;
      },
      set: (key, value) => backing.set(key, value),
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    const contender = journal({ storage: interleaved, scope: () => scope, now: () => currentTime });

    expect(() => contender.reserve("goals.resume", { sessionId: "ses_renewed" })).toThrow(
      MutationJournalOwnershipError,
    );
    expect(backing.get(expiredLeaseKey)).toBeUndefined();
    expect(backing.get(renewedLeaseKey)).toMatchObject({ expiresAt: 61_001 });
    expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
      owner: "renewing-renderer",
    });
  });

  it("hands unresolved identities to a replacement client and fences the retired settlement", async () => {
    const storage = new MemoryStorage();
    const params = { sessionId: "ses_1" };
    const retired = journal({ storage, scope: () => scope });
    const original = retired.reserve("goals.resume", params)!;
    let resolveRetired!: (value: string) => void;
    const retiredMutation = opening(
      original,
      () =>
        new Promise<string>((resolve) => {
          resolveRetired = resolve;
        }),
    );
    await vi.waitFor(() => expect(resolveRetired).toBeTypeOf("function"));

    retired.dispose();
    const replacement = journal({ storage, scope: () => scope });
    const inherited = replacement.reserve("goals.resume", params)!;
    expect(inherited.idempotencyKey).toBe(original.idempotencyKey);
    expect(persistedEntry(storage, original.idempotencyKey)).toMatchObject({ claimable: false });

    resolveRetired("retired response");
    await expect(retiredMutation).resolves.toBe("retired response");
    expect(persistedEntry(storage, original.idempotencyKey)).toMatchObject({
      idempotencyKey: original.idempotencyKey,
      claimable: false,
    });
  });

  it("does not let a superseded renderer rewrite its successor's journal entry", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const reservation = created.reserve("sessions.delete", { sessionId: "ses_1" })!;
    const rejectRetired: Array<(error: unknown) => void> = [];
    const retiredMutation = opening(
      reservation,
      () =>
        new Promise<string>((_resolve, reject) => {
          rejectRetired.push(reject);
        }),
    );
    await vi.waitFor(() => expect(rejectRetired).toHaveLength(1));

    const successor = persistedEntry(storage, reservation.idempotencyKey);
    successor.owner = "successor-renderer";
    successor.claimable = false;
    storage.set(persistedEntryKey(storage, reservation.idempotencyKey), successor);

    const failure = new RpcTransportError("retired response lost");
    rejectRetired[0]!(failure);
    await vi.waitFor(() => expect(rejectRetired).toHaveLength(2));
    rejectRetired[1]!(failure);
    await expect(retiredMutation).rejects.toBe(failure);
    expect(persistedEntry(storage, reservation.idempotencyKey)).toMatchObject({
      owner: "successor-renderer",
      claimable: false,
    });
  });

  it("removes determinate outcomes and retains ambiguous ones for explicit retry", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };
    const ambiguous = created.reserve("goals.stop", params)!;
    const failure = new RpcTransportError("response lost");

    await expect(opening(ambiguous, () => Promise.reject(failure))).rejects.toBe(failure);
    const liveTwin = journal({ storage, scope: () => scope }).reserve("goals.stop", params)!;
    expect(liveTwin.idempotencyKey).not.toBe(ambiguous.idempotencyKey);
    expect(created.reserve("goals.stop", params)?.idempotencyKey).toBe(ambiguous.idempotencyKey);

    const definitive = created.reserve("goals.resume", params)!;
    const refusal = new RpcError({
      message: "paused goal missing",
      data: { type: "session_busy" },
    });
    await expect(opening(definitive, () => Promise.reject(refusal))).rejects.toBe(refusal);
    expect(created.reserve("goals.resume", params)?.idempotencyKey).not.toBe(
      definitive.idempotencyKey,
    );

    const transportRefusal = created.reserve("sessions.delete", params)!;
    const rejectedBeforeAdmission = new RpcTransportError("bad request", 400);
    await expect(
      opening(transportRefusal, () => Promise.reject(rejectedBeforeAdmission)),
    ).rejects.toBe(rejectedBeforeAdmission);
    expect(created.reserve("sessions.delete", params)?.idempotencyKey).not.toBe(
      transportRefusal.idempotencyKey,
    );
  });

  it("removes a successful command and keeps the same key on MutationPromise.retry", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const reservation = created.reserve("sessions.delete", { sessionId: "ses_1" })!;
    const execute = vi.fn().mockResolvedValue("deleted");
    const mutation = opening(reservation, execute);

    await expect(mutation).resolves.toBe("deleted");
    expect(storage.entryValues()).toHaveLength(0);
    const retry = mutation.retry();
    expect(retry.idempotencyKey).toBe(reservation.idempotencyKey);
    await expect(retry).resolves.toBe("deleted");
    expect(storage.entryValues()).toHaveLength(0);
  });

  it("does not execute retry until a failed ownership claim is durable", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const reservation = created.reserve("goals.stop", { sessionId: "ses_1" })!;
    const failure = new RpcTransportError("response lost");
    const execute = vi.fn().mockRejectedValue(failure);
    const mutation = opening(reservation, execute);
    await expect(mutation).rejects.toBe(failure);
    expect(execute).toHaveBeenCalledTimes(2);

    storage.failSet = true;
    const failedRetry = mutation.retry();
    await expect(failedRetry).rejects.toBeInstanceOf(MutationJournalStorageError);
    expect(execute).toHaveBeenCalledTimes(2);

    storage.failSet = false;
    execute.mockResolvedValue("stopped");
    const recovered = failedRetry.retry();
    expect(recovered.idempotencyKey).toBe(reservation.idempotencyKey);
    await expect(recovered).resolves.toBe("stopped");
    expect(execute).toHaveBeenCalledTimes(3);
  });

  it("recovers an entry whose durable write reported failure without exposing it to a twin", () => {
    const backing = new MemoryStorage();
    let throwAfterEntryWrite = true;
    const storage: MutationJournalStorage = {
      get: (key) => backing.get(key),
      set: (key, value) => {
        backing.set(key, value);
        if (throwAfterEntryWrite && key.startsWith("entry:")) {
          throwAfterEntryWrite = false;
          throw new Error("write confirmation failed");
        }
      },
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    const created = journal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };

    expect(() => created.reserve("goals.stop", params, "stable-key")).toThrow(
      MutationJournalStorageError,
    );
    expect(backing.entryValues()).toHaveLength(1);
    expect(created.reserve("goals.stop", params, "stable-key")?.idempotencyKey).toBe("stable-key");
    expect(created.reserve("goals.stop", params, "twin-key")?.idempotencyKey).toBe("twin-key");
  });

  it("removes a confirmed-unsent entry when its journal is disposed", () => {
    const backing = new MemoryStorage();
    let throwAfterEntryWrite = true;
    const storage: MutationJournalStorage = {
      get: (key) => backing.get(key),
      set: (key, value) => {
        backing.set(key, value);
        if (throwAfterEntryWrite && key.startsWith("entry:")) {
          throwAfterEntryWrite = false;
          throw new Error("write confirmation failed");
        }
      },
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    const created = journal({ storage, scope: () => scope });

    expect(() => created.reserve("goals.stop", { sessionId: "ses_1" }, "unsent-key")).toThrow(
      MutationJournalStorageError,
    );
    expect(backing.entryValues()).toHaveLength(1);
    created.dispose();
    expect(backing.entryValues()).toHaveLength(0);
  });

  it("fails the business result when cleanup is not durable and reuses its key after recovery", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };
    const reservation = created.reserve("sessions.delete", params)!;
    storage.failRemove = true;

    await expect(opening(reservation, () => Promise.resolve("deleted"))).rejects.toBeInstanceOf(
      MutationJournalStorageError,
    );
    storage.failRemove = false;
    expect(created.reserve("sessions.delete", params)?.idempotencyKey).toBe(
      reservation.idempotencyKey,
    );
  });

  it("retains the key in memory when persisting an ambiguous outcome fails", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };
    const reservation = created.reserve("goals.stop", params)!;
    const failure = new RpcTransportError("response lost");
    storage.failSet = true;

    await expect(opening(reservation, () => Promise.reject(failure))).rejects.toBeInstanceOf(
      MutationJournalStorageError,
    );
    storage.failSet = false;
    expect(created.reserve("goals.stop", params)?.idempotencyKey).toBe(reservation.idempotencyKey);
  });

  it("fails closed across a replaced Runtime store, another endpoint, and shorter retention", () => {
    const storage = new MemoryStorage();
    let currentTime = 1_000;
    const firstJournal = journal({ storage, scope: () => scope, now: () => currentTime });
    const original = firstJournal.reserve("sessions.create", { title: "same" })!;

    const replaced = journal({
      storage,
      scope: () => ({ ...scope, namespace: "idp_store_b" }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(replaced.idempotencyKey).not.toBe(original.idempotencyKey);

    const otherEndpoint = journal({
      storage,
      scope: () => ({ ...scope, endpoint: "http://127.0.0.1:27171" }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(otherEndpoint.idempotencyKey).not.toBe(replaced.idempotencyKey);

    currentTime += 1_000;
    const expiredByShrink = journal({
      storage,
      scope: () => ({
        ...scope,
        endpoint: "http://127.0.0.1:27171",
        retentionSeconds: 1,
      }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(expiredByShrink.idempotencyKey).not.toBe(otherEndpoint.idempotencyKey);
  });

  it("does not let stale retention cleanup delete a generation appended after its snapshot", () => {
    const backing = new MemoryStorage();
    let currentTime = 1_000;
    let retentionSeconds = 86_400;
    let armed = false;
    let keyReads = 0;
    let successor: Record<string, unknown> | undefined;
    const storage: MutationJournalStorage = {
      get: (key) => backing.get(key),
      set: (key, value) => backing.set(key, value),
      remove: (key) => backing.remove(key),
      keys() {
        if (armed && ++keyReads === 3) {
          const next = successor!;
          const key = `entry:${encodeURIComponent(String(next.idempotencyKey))}:${String(next.generation)}:${encodeURIComponent(String(next.generationId))}`;
          backing.set(key, next);
        }
        return backing.keys();
      },
    };
    const created = journal({
      storage,
      scope: () => ({ ...scope, retentionSeconds }),
      now: () => currentTime,
    });
    const original = created.reserve("sessions.create", { title: "first" })!;
    const originalEntry = persistedEntry(backing, original.idempotencyKey);
    successor = {
      ...originalEntry,
      generation: Number(originalEntry.generation) + 1,
      generationId: "successor-generation",
      owner: "successor-renderer",
      claimable: false,
    };
    currentTime = 2_001;
    retentionSeconds = 1;
    armed = true;

    created.reserve("sessions.create", { title: "unrelated" });

    expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
      generation: 1,
      generationId: "successor-generation",
      owner: "successor-renderer",
    });
  });

  it("migrates the shipped v1 snapshot once without discarding unresolved identities", () => {
    const legacyKey = "legacy-idempotency-key";
    const storage = new MemoryStorage({
      version: 1,
      salt: "legacy-salt",
      entries: [
        {
          endpoint: "http://127.0.0.1:17171",
          namespace: "idp_store_a",
          fingerprint: "0".repeat(32),
          idempotencyKey: legacyKey,
          owner: "old-process",
          claimable: false,
          createdAt: Date.now() - 1_000,
          expiresAt: Date.now() + 86_400_000,
        },
      ],
    });

    journal({ storage, scope: () => scope }).reserve("sessions.delete", { sessionId: "other" });

    expect(storage.get("legacy:v1")).toBeUndefined();
    expect(persistedEntry(storage, legacyKey)).toMatchObject({
      version: 3,
      salt: "legacy-salt",
      idempotencyKey: legacyKey,
    });
  });

  it("migrates shipped v2 entry records into immutable generation zero", () => {
    const storage = new MemoryStorage();
    const idempotencyKey = "shipped-v2-key";
    storage.set(`entry:${encodeURIComponent(idempotencyKey)}`, {
      version: 2,
      salt: "shipped-v2-salt",
      endpoint: "http://127.0.0.1:17171",
      namespace: "idp_store_a",
      fingerprint: "0".repeat(32),
      idempotencyKey,
      owner: "old-process",
      claimable: false,
      createdAt: 1_000,
      expiresAt: 86_401_000,
    });

    journal({ storage, scope: () => scope, now: () => 2_000 }).reserve("sessions.delete", {
      sessionId: "other",
    });

    expect(storage.get(`entry:${encodeURIComponent(idempotencyKey)}`)).toBeUndefined();
    expect(persistedEntry(storage, idempotencyKey)).toMatchObject({
      version: 3,
      salt: "shipped-v2-salt",
      generation: 0,
      generationId: "shipped-v2-salt",
      settled: false,
      idempotencyKey,
    });
  });

  it("resumes an interrupted v1 migration record by record", () => {
    const createdAt = Date.now() - 1_000;
    const legacyEntry = (idempotencyKey: string) => ({
      endpoint: "http://127.0.0.1:17171",
      namespace: "idp_store_a",
      fingerprint: "0".repeat(32),
      idempotencyKey,
      owner: "old-process",
      claimable: false,
      createdAt,
      expiresAt: Date.now() + 86_400_000,
    });
    const storage = new MemoryStorage({
      version: 1,
      salt: "legacy-salt",
      entries: [legacyEntry("legacy-key-1"), legacyEntry("legacy-key-2")],
    });
    storage.failSetKey = "entry:legacy-key-2";

    expect(() =>
      journal({ storage, scope: () => scope }).reserve("sessions.delete", { sessionId: "other" }),
    ).toThrow(MutationJournalStorageError);
    expect(persistedEntry(storage, "legacy-key-1")).toBeDefined();
    expect(storage.get("legacy:v1")).toBeDefined();

    storage.failSetKey = undefined;
    journal({ storage, scope: () => scope }).reserve("sessions.delete", { sessionId: "other" });
    expect(persistedEntry(storage, "legacy-key-1")).toBeDefined();
    expect(persistedEntry(storage, "legacy-key-2")).toBeDefined();
    expect(storage.get("legacy:v1")).toBeUndefined();
  });

  it("refreshes live ownership and stops the heartbeat on dispose", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-13T00:00:00Z"));
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    created.reserve("sessions.delete", { sessionId: "ses_1" });
    const firstOwnerKey = storage.keys().find((key) => key.startsWith("owner:"));
    expect(firstOwnerKey).toBeDefined();
    const firstExpiry = (storage.get(firstOwnerKey!) as { expiresAt: number }).expiresAt;

    await vi.advanceTimersByTimeAsync(10_000);
    const refreshedOwnerKeys = storage.keys().filter((key) => key.startsWith("owner:"));
    expect(refreshedOwnerKeys).toHaveLength(1);
    expect(refreshedOwnerKeys).not.toContain(firstOwnerKey);
    expect(storage.get(firstOwnerKey!)).toBeUndefined();
    const refreshedOwnerKey = refreshedOwnerKeys[0]!;
    const refreshedExpiry = (storage.get(refreshedOwnerKey) as { expiresAt: number }).expiresAt;
    expect(refreshedExpiry).toBeGreaterThan(firstExpiry);

    created.dispose();
    await vi.advanceTimersByTimeAsync(20_000);
    expect(storage.get(refreshedOwnerKey)).toBeUndefined();
    expect(storage.keys().filter((key) => key.startsWith("owner:"))).toEqual([]);
  });

  it("reclaims every owned generation when a heartbeat could not remove its predecessor", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-13T00:00:00Z"));
    const backing = new MemoryStorage();
    let failNextOwnerRemoval = false;
    const storage: MutationJournalStorage = {
      get: (key) => backing.get(key),
      set: (key, value) => backing.set(key, value),
      remove(key) {
        if (key.startsWith("owner:") && failNextOwnerRemoval) {
          failNextOwnerRemoval = false;
          throw new Error("interrupted generation cleanup");
        }
        backing.remove(key);
      },
      keys: () => backing.keys(),
    };
    const created = journal({ storage, scope: () => scope });
    created.reserve("sessions.delete", { sessionId: "ses_1" });
    failNextOwnerRemoval = true;

    await vi.advanceTimersByTimeAsync(10_000);
    expect(backing.keys().filter((key) => key.startsWith("owner:"))).toHaveLength(2);

    created.dispose();
    expect(backing.keys().filter((key) => key.startsWith("owner:"))).toEqual([]);
  });

  it("keeps the process lease until the last journal sharing that storage retires", () => {
    const storage = new MemoryStorage();
    const first = journal({ storage, scope: () => scope });
    const second = journal({ storage, scope: () => scope });
    first.reserve("sessions.delete", { sessionId: "ses_1" });
    const firstOwnerKey = storage.keys().find((key) => key.startsWith("owner:"));
    expect(firstOwnerKey).toBeDefined();
    second.reserve("sessions.delete", { sessionId: "ses_2" });
    const ownerKeys = storage.keys().filter((key) => key.startsWith("owner:"));
    expect(ownerKeys).toHaveLength(2);
    const secondOwnerKey = ownerKeys.find((key) => key !== firstOwnerKey);
    expect(secondOwnerKey).toBeDefined();

    first.dispose();
    expect(storage.get(firstOwnerKey!)).toBeUndefined();
    expect(storage.get(secondOwnerKey!)).toBeDefined();

    second.dispose();
    expect(storage.get(secondOwnerKey!)).toBeUndefined();
  });

  it("fences a late settlement after a distinct renderer takes over an expired lease", async () => {
    const storage = new MemoryStorage();
    let currentTime = 1_000;
    vi.resetModules();
    const firstContext = await import("./mutationJournal");
    const first = firstContext.createMutationJournal({
      storage,
      scope: () => scope,
      now: () => currentTime,
    });
    vi.resetModules();
    const secondContext = await import("./mutationJournal");
    const second = secondContext.createMutationJournal({
      storage,
      scope: () => scope,
      now: () => currentTime,
    });
    const params = { sessionId: "ses_cross_renderer" };
    let settleRetired!: (value: string) => void;

    try {
      const original = first.reserve("goals.resume", params)!;
      const originalOwner = persistedEntry(storage, original.idempotencyKey).owner;
      const retired = original.track(
        createMutationPromise(
          () =>
            new Promise<string>((resolve) => {
              settleRetired = resolve;
            }),
          original.idempotencyKey,
        ),
      );
      await vi.waitFor(() => expect(settleRetired).toBeTypeOf("function"));

      expect(() => second.reserve("goals.resume", params)).toThrow(
        secondContext.MutationJournalOwnershipError,
      );

      currentTime = 31_001;
      const inherited = second.reserve("goals.resume", params)!;
      expect(inherited.idempotencyKey).toBe(original.idempotencyKey);
      const successor = persistedEntry(storage, inherited.idempotencyKey);
      expect(successor.owner).not.toBe(originalOwner);

      settleRetired("late retired success");
      await expect(retired).resolves.toBe("late retired success");
      expect(persistedEntry(storage, inherited.idempotencyKey)).toMatchObject({
        owner: successor.owner,
        claimable: false,
      });

      await expect(
        inherited.track(
          createMutationPromise(
            () => Promise.resolve("successor replay"),
            inherited.idempotencyKey,
          ),
        ),
      ).resolves.toBe("successor replay");
      expect(storage.entryValues()).toEqual([]);
    } finally {
      first.dispose();
      second.dispose();
    }
  });

  it("does not let closing overwrite a successor that takes over after the owner read", async () => {
    const backing = new MemoryStorage();
    let currentTime = 1_000;
    let armTakeover = false;
    let takeoverInjected = false;
    let replacement: MutationJournal | undefined;
    let inherited: ReturnType<MutationJournal["reserve"]>;
    const params = { sessionId: "ses_close_interleave" };
    const firstStorage: MutationJournalStorage = {
      get(key) {
        const value = backing.get(key);
        if (armTakeover && !takeoverInjected && key.startsWith("entry:")) {
          takeoverInjected = true;
          inherited = replacement!.reserve("goals.resume", params);
        }
        return value;
      },
      set: (key, value) => backing.set(key, value),
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    vi.resetModules();
    const firstContext = await import("./mutationJournal");
    const first = firstContext.createMutationJournal({
      storage: firstStorage,
      scope: () => scope,
      now: () => currentTime,
    });
    vi.resetModules();
    const secondContext = await import("./mutationJournal");
    replacement = secondContext.createMutationJournal({
      storage: backing,
      scope: () => scope,
      now: () => currentTime,
    });

    try {
      const original = first.reserve("goals.resume", params)!;
      const originalOwner = persistedEntry(backing, original.idempotencyKey).owner;
      currentTime = 31_001;
      armTakeover = true;

      first.dispose();

      expect(inherited?.idempotencyKey).toBe(original.idempotencyKey);
      expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
        idempotencyKey: original.idempotencyKey,
        claimable: false,
      });
      expect(persistedEntry(backing, original.idempotencyKey).owner).not.toBe(originalOwner);
    } finally {
      replacement.dispose();
    }
  });

  it("does not let a late success delete a successor that takes over after the owner read", async () => {
    const backing = new MemoryStorage();
    let currentTime = 1_000;
    let armTakeover = false;
    let takeoverInjected = false;
    let replacement: MutationJournal | undefined;
    let inherited: ReturnType<MutationJournal["reserve"]>;
    const params = { sessionId: "ses_settlement_interleave" };
    const firstStorage: MutationJournalStorage = {
      get(key) {
        const value = backing.get(key);
        if (armTakeover && !takeoverInjected && key.startsWith("entry:")) {
          takeoverInjected = true;
          inherited = replacement!.reserve("goals.resume", params);
        }
        return value;
      },
      set: (key, value) => backing.set(key, value),
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    vi.resetModules();
    const firstContext = await import("./mutationJournal");
    const first = firstContext.createMutationJournal({
      storage: firstStorage,
      scope: () => scope,
      now: () => currentTime,
    });
    vi.resetModules();
    const secondContext = await import("./mutationJournal");
    replacement = secondContext.createMutationJournal({
      storage: backing,
      scope: () => scope,
      now: () => currentTime,
    });
    let settleRetired!: (value: string) => void;

    try {
      const original = first.reserve("goals.resume", params)!;
      const retired = original.track(
        createMutationPromise(
          () =>
            new Promise<string>((resolve) => {
              settleRetired = resolve;
            }),
          original.idempotencyKey,
        ),
      );
      await vi.waitFor(() => expect(settleRetired).toBeTypeOf("function"));
      currentTime = 31_001;
      armTakeover = true;

      settleRetired("late retired success");
      await expect(retired).resolves.toBe("late retired success");

      expect(inherited?.idempotencyKey).toBe(original.idempotencyKey);
      expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
        idempotencyKey: original.idempotencyKey,
        claimable: false,
      });
    } finally {
      first.dispose();
      replacement.dispose();
    }
  });

  it("generation-fences a same-renderer replacement that takes over during settlement", async () => {
    const backing = new MemoryStorage();
    const params = { sessionId: "ses_same_renderer_interleave" };
    let armReplacement = false;
    let replacementInjected = false;
    let retired: MutationJournal | undefined;
    let replacement: MutationJournal | undefined;
    let inherited: ReturnType<MutationJournal["reserve"]>;
    const storage: MutationJournalStorage = {
      get(key) {
        const value = backing.get(key);
        if (armReplacement && !replacementInjected && key.startsWith("entry:")) {
          replacementInjected = true;
          retired!.dispose();
          inherited = replacement!.reserve("goals.resume", params);
        }
        return value;
      },
      set: (key, value) => backing.set(key, value),
      remove: (key) => backing.remove(key),
      keys: () => backing.keys(),
    };
    retired = journal({ storage, scope: () => scope });
    replacement = journal({ storage, scope: () => scope });
    let settleRetired!: (value: string) => void;

    const original = retired.reserve("goals.resume", params)!;
    const pending = original.track(
      createMutationPromise(
        () =>
          new Promise<string>((resolve) => {
            settleRetired = resolve;
          }),
        original.idempotencyKey,
      ),
    );
    await vi.waitFor(() => expect(settleRetired).toBeTypeOf("function"));
    armReplacement = true;

    settleRetired("retired success");
    await expect(pending).resolves.toBe("retired success");

    expect(inherited?.idempotencyKey).toBe(original.idempotencyKey);
    expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
      generation: 1,
      claimable: false,
      settled: false,
    });

    await expect(
      inherited!.track(
        createMutationPromise(
          () => Promise.resolve("replacement success"),
          inherited!.idempotencyKey,
        ),
      ),
    ).resolves.toBe("replacement success");
    expect(persistedEntry(backing, original.idempotencyKey)).toMatchObject({
      generation: 1,
      claimable: false,
      settled: true,
    });
  });

  it("keeps a successor settlement fenced against an older renderer's later failure", async () => {
    const storage = new MemoryStorage();
    let currentTime = 1_000;
    vi.resetModules();
    const firstContext = await import("./mutationJournal");
    const first = firstContext.createMutationJournal({
      storage,
      scope: () => scope,
      now: () => currentTime,
    });
    vi.resetModules();
    const secondContext = await import("./mutationJournal");
    const second = secondContext.createMutationJournal({
      storage,
      scope: () => scope,
      now: () => currentTime,
    });
    const params = { sessionId: "ses_settled_fence" };
    const rejectRetired: Array<(error: unknown) => void> = [];

    try {
      const original = first.reserve("goals.resume", params)!;
      const retired = original.track(
        createMutationPromise(
          () =>
            new Promise<string>((_resolve, reject) => {
              rejectRetired.push(reject);
            }),
          original.idempotencyKey,
        ),
      );
      await vi.waitFor(() => expect(rejectRetired).toHaveLength(1));
      currentTime = 31_001;
      const inherited = second.reserve("goals.resume", params)!;

      await expect(
        inherited.track(
          createMutationPromise(
            () => Promise.resolve("successor settled"),
            inherited.idempotencyKey,
          ),
        ),
      ).resolves.toBe("successor settled");
      expect(persistedEntry(storage, inherited.idempotencyKey)).toMatchObject({
        generation: 1,
        settled: true,
        claimable: false,
      });

      const lostResponse = new RpcTransportError("retired response lost");
      rejectRetired[0]!(lostResponse);
      await vi.waitFor(() => expect(rejectRetired).toHaveLength(2));
      rejectRetired[1]!(lostResponse);
      await expect(retired).rejects.toBe(lostResponse);
      first.dispose();
      expect(persistedEntry(storage, inherited.idempotencyKey)).toMatchObject({
        generation: 1,
        settled: true,
        claimable: false,
      });

      const fresh = second.reserve("goals.resume", params)!;
      expect(fresh.idempotencyKey).not.toBe(inherited.idempotencyKey);
    } finally {
      first.dispose();
      second.dispose();
    }
  });

  it("hands off through process memory when disposal persistence fails", () => {
    const storage = new MemoryStorage();
    const firstStorage: MutationJournalStorage = {
      get: (key) => storage.get(key),
      set: (key, value) => storage.set(key, value),
      remove: (key) => storage.remove(key),
      keys: () => storage.keys(),
    };
    const first = journal({ storage: firstStorage, scope: () => scope });
    const original = first.reserve("goals.stop", { sessionId: "ses_1" })!;
    storage.failSet = true;
    expect(() => first.dispose()).toThrow(MutationJournalStorageError);

    storage.failSet = false;
    const replacementStorage: MutationJournalStorage = {
      get: (key) => storage.get(key),
      set: (key, value) => storage.set(key, value),
      remove: (key) => storage.remove(key),
      keys: () => storage.keys(),
    };
    const replacement = journal({ storage: replacementStorage, scope: () => scope });
    expect(replacement.reserve("goals.stop", { sessionId: "ses_1" })?.idempotencyKey).toBe(
      original.idempotencyKey,
    );
  });

  it("fails closed on malformed state, unavailable storage, and exhausted capacity", () => {
    const malformed = new MemoryStorage({
      version: 1,
      salt: "salt",
      entries: [{ nope: true }],
    });
    const undiscovered = journal({ storage: malformed, scope: () => null });
    expect(undiscovered.reserve("sessions.delete", { sessionId: "ses_1" })).toBeUndefined();
    expect(() =>
      journal({ storage: malformed, scope: () => scope }).reserve("sessions.delete", {
        sessionId: "ses_1",
      }),
    ).toThrow(MutationJournalStorageError);

    const unavailable = new MemoryStorage();
    unavailable.failSet = true;
    expect(() =>
      journal({ storage: unavailable, scope: () => scope }).reserve("sessions.delete", {
        sessionId: "ses_1",
      }),
    ).toThrow(MutationJournalStorageError);

    const full = new MemoryStorage();
    const fullJournal = journal({ storage: full, scope: () => scope });
    for (let index = 0; index < 256; index++) {
      fullJournal.reserve("sessions.delete", { sessionId: `ses_${index}` });
    }
    expect(() => fullJournal.reserve("sessions.delete", { sessionId: "ses_overflow" })).toThrow(
      MutationJournalCapacityError,
    );
    expect(full.entryValues()).toHaveLength(256);
  });
});
