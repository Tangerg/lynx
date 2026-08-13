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
    return [...this.values.entries()]
      .filter(([key]) => key.startsWith("entry:"))
      .map(([, value]) => value as Record<string, unknown>);
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
  const entry = storage.get(`entry:${encodeURIComponent(idempotencyKey)}`);
  expect(entry).toBeDefined();
  return entry as Record<string, unknown>;
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
    crashImage.set(`entry:${encodeURIComponent(original.idempotencyKey)}`, persisted);

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
    storage.set(`entry:${encodeURIComponent(first.idempotencyKey)}`, persisted);
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

  it("removes determinate outcomes and retains ambiguous ones for explicit retry", async () => {
    const storage = new MemoryStorage();
    const created = journal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };
    const ambiguous = created.reserve("goals.stop", params)!;
    const failure = new RpcTransportError("response lost");

    await expect(opening(ambiguous, () => Promise.reject(failure))).rejects.toBe(failure);
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
      version: 2,
      salt: "legacy-salt",
      idempotencyKey: legacyKey,
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
    const ownerKey = storage.keys().find((key) => key.startsWith("owner:"));
    expect(ownerKey).toBeDefined();
    const firstExpiry = (storage.get(ownerKey!) as { expiresAt: number }).expiresAt;

    await vi.advanceTimersByTimeAsync(10_000);
    const refreshedExpiry = (storage.get(ownerKey!) as { expiresAt: number }).expiresAt;
    expect(refreshedExpiry).toBeGreaterThan(firstExpiry);

    created.dispose();
    await vi.advanceTimersByTimeAsync(20_000);
    expect((storage.get(ownerKey!) as { expiresAt: number }).expiresAt).toBe(refreshedExpiry);
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
