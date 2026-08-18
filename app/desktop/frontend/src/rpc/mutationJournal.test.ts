import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createMutationPromise, type MutationPromise } from "./mutation";
import {
  createMutationJournal,
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
  type MutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
  type MutationReservation,
} from "./mutationJournal";

class MemoryStorage implements MutationJournalStorage {
  readonly values = new Map<string, unknown>();

  get(key: string): unknown {
    const value = this.values.get(key);
    return value === undefined ? undefined : structuredClone(value);
  }

  set(key: string, value: unknown): void {
    this.values.set(key, structuredClone(value));
  }

  remove(key: string): void {
    this.values.delete(key);
  }

  keys(): string[] {
    return [...this.values.keys()];
  }
}

function scope(namespace = "idp_runtime_store"): MutationJournalScope {
  return {
    namespace,
    retentionSeconds: 86_400,
  };
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve(value: T): void;
  reject(error: unknown): void;
} {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function deliver<T>(
  reservation: MutationReservation,
  execute: () => Promise<T> | T,
): MutationPromise<T> {
  return reservation.track(
    createMutationPromise(async () => {
      reservation.authorizeAttempt();
      return execute();
    }, reservation.idempotencyKey),
  );
}

const journals: MutationJournal[] = [];

function journal(
  storage: MutationJournalStorage,
  currentScope: () => MutationJournalScope | null | undefined = () => scope(),
): MutationJournal {
  const created = createMutationJournal({ storage, scope: currentScope });
  journals.push(created);
  return created;
}

afterEach(() => {
  for (const current of journals.splice(0).reverse()) current.dispose();
});

describe("mutation journal", () => {
  it("persists only the unresolved command identity in the current shape", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const reservation = current.reserve("runs.start", { prompt: "never persist me" })!;

    expect(storage.keys()).toHaveLength(1);
    const entry = storage.get(storage.keys()[0]!) as Record<string, unknown>;
    expect(Object.keys(entry).sort()).toEqual([
      "createdAt",
      "expiresAt",
      "fingerprint",
      "idempotencyKey",
      "namespace",
      "salt",
      "version",
    ]);
    expect(JSON.stringify(entry)).not.toContain("never persist me");
    expect(entry).not.toHaveProperty("owner");
    expect(entry).not.toHaveProperty("generation");
    expect(entry).not.toHaveProperty("claimable");
    expect(entry).not.toHaveProperty("settled");

    await expect(deliver(reservation, () => "ok")).resolves.toBe("ok");
    expect(storage.keys()).toEqual([]);
  });

  it("uses process-local claims to separate concurrent identical commands", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const first = current.reserve("schedules.runNow", { scheduleId: "schedule_1" })!;
    const second = current.reserve("schedules.runNow", { scheduleId: "schedule_1" })!;

    expect(second.idempotencyKey).not.toBe(first.idempotencyKey);
    expect(storage.keys()).toHaveLength(2);
    await Promise.all([deliver(first, () => undefined), deliver(second, () => undefined)]);
    expect(storage.keys()).toEqual([]);
  });

  it("publishes a successor before fencing every late predecessor settlement", async () => {
    const storage = new MemoryStorage();
    const predecessor = journal(storage);
    const firstReservation = predecessor.reserve("runs.start", { prompt: "hello" })!;
    const firstResult = deferred<string>();
    const first = deliver(firstReservation, () => firstResult.promise);
    await vi.waitFor(() => expect(storage.keys()).toHaveLength(1));

    const successor = journal(storage);
    const replayReservation = successor.reserve("runs.start", { prompt: "hello" })!;
    expect(replayReservation.idempotencyKey).toBe(firstReservation.idempotencyKey);
    const replayResult = deferred<string>();
    const replay = deliver(replayReservation, () => replayResult.promise);

    firstResult.resolve("late predecessor result");
    await expect(first).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    expect(storage.keys()).toHaveLength(1);
    predecessor.dispose();
    expect(storage.keys()).toHaveLength(1);

    replayResult.resolve("successor result");
    await expect(replay).resolves.toBe("successor result");
    expect(storage.keys()).toEqual([]);
  });

  it("reuses an unresolved identity when the same Runtime store changes transport binding", () => {
    const storage = new MemoryStorage();
    const bind = (endpoint: string) => ({ endpoint, journal: journal(storage) });
    const predecessor = bind("http://127.0.0.1:17171");
    const pending = predecessor.journal.reserve("runs.start", { prompt: "hello" })!;

    const successor = bind("http://127.0.0.1:27272");
    const replay = successor.journal.reserve("runs.start", { prompt: "hello" })!;

    expect(successor.endpoint).not.toBe(predecessor.endpoint);
    expect(replay.idempotencyKey).toBe(pending.idempotencyKey);
    expect(storage.keys()).toHaveLength(1);
  });

  it("retains an ambiguous result and reuses its exact identity on explicit retry", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const reservation = current.reserve("runs.start", { prompt: "hello" })!;
    const execute = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockResolvedValueOnce("replayed");
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(RpcTransportError);
    expect(storage.keys()).toHaveLength(1);
    const replay = first.retry();
    expect(replay.idempotencyKey).toBe(first.idempotencyKey);
    await expect(replay).resolves.toBe("replayed");
    expect(execute).toHaveBeenCalledTimes(3);
    expect(storage.keys()).toEqual([]);
  });

  it("keeps an unresolved identity while Runtime scope is temporarily unavailable", async () => {
    const storage = new MemoryStorage();
    let available = true;
    const current = journal(storage, () => (available ? scope() : null));
    const reservation = current.reserve("runs.start", { prompt: "hello" })!;
    const execute = vi.fn(async () => {
      available = false;
      throw new RpcTransportError("discovery withdrawn");
    });
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(MutationJournalScopeUnavailableError);
    expect(storage.keys()).toHaveLength(1);
    available = true;
    execute.mockResolvedValueOnce(undefined as never);
    await expect(first.retry()).resolves.toBeUndefined();
    expect(storage.keys()).toEqual([]);
  });

  it("retires identities from a replaced Runtime store instead of replaying them", () => {
    const storage = new MemoryStorage();
    let namespace = "idp_runtime_store_a";
    const predecessor = journal(storage, () => scope(namespace));
    const old = predecessor.reserve("runs.start", { prompt: "hello" })!;

    namespace = "idp_runtime_store_b";
    expect(() => old.authorizeAttempt()).toThrow(MutationJournalOwnershipError);
    const successor = journal(storage, () => scope(namespace));
    const replacement = successor.reserve("runs.start", { prompt: "hello" })!;

    expect(replacement.idempotencyKey).not.toBe(old.idempotencyKey);
    expect(storage.keys()).toHaveLength(1);
  });

  it("fails closed on corrupted records, identity collisions, and capacity", () => {
    const corrupted = new MemoryStorage();
    corrupted.values.set("entry:broken", { version: 1 });
    expect(() => journal(corrupted).reserve("runs.start", {})).toThrow(MutationJournalStorageError);

    const endpointShape = new MemoryStorage();
    endpointShape.values.set("entry:old-key", {
      version: 2,
      salt: "salt",
      endpoint: "http://127.0.0.1:17171",
      namespace: "idp_runtime_store",
      fingerprint: "0".repeat(32),
      idempotencyKey: "old-key",
      createdAt: 1,
      expiresAt: 2,
    });
    expect(() => journal(endpointShape).reserve("runs.start", {})).toThrow(
      MutationJournalStorageError,
    );

    const storage = new MemoryStorage();
    const current = journal(storage);
    current.reserve("runs.start", { prompt: "first" }, "fixed-key");
    expect(() => current.reserve("runs.start", { prompt: "second" }, "fixed-key")).toThrow(
      MutationJournalOwnershipError,
    );
    for (let index = 1; index < 256; index++) {
      current.reserve(`runs.command.${index}`, { index });
    }
    expect(() => current.reserve("runs.command.overflow", {})).toThrow(
      MutationJournalCapacityError,
    );
  });

  it("surfaces cleanup failure without losing the retryable durable identity", async () => {
    const storage = new MemoryStorage();
    let rejectRemoval = true;
    storage.remove = (key: string) => {
      if (rejectRemoval) throw new Error("disk unavailable");
      storage.values.delete(key);
    };
    const current = journal(storage);
    const reservation = current.reserve("runs.start", { prompt: "hello" })!;
    const execute = vi.fn().mockResolvedValue("done");
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(MutationJournalStorageError);
    expect(storage.keys()).toHaveLength(1);
    rejectRemoval = false;
    await expect(first.retry()).resolves.toBe("done");
    expect(execute).toHaveBeenCalledTimes(2);
    expect(storage.keys()).toEqual([]);
  });

  it("does not schedule ownership heartbeats and disposal is idempotent", () => {
    const interval = vi.spyOn(globalThis, "setInterval");
    const storage = new MemoryStorage();
    const current = journal(storage);
    current.reserve("runs.start", { prompt: "hello" });
    const persisted = structuredClone([...storage.values.entries()]);

    current.dispose();
    current.dispose();

    expect(interval).not.toHaveBeenCalled();
    expect([...storage.values.entries()]).toEqual(persisted);
    interval.mockRestore();
  });
});
