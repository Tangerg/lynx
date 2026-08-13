import { describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "./errors";
import { createMutationPromise } from "./mutation";
import {
  createMutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
} from "./mutationJournal";

function memoryStorage(initial?: unknown): MutationJournalStorage & { snapshot: () => unknown } {
  let value = initial;
  return {
    read: () => value,
    write: (next) => {
      value = structuredClone(next);
    },
    snapshot: () => value,
  };
}

const scope: MutationJournalScope = {
  endpoint: "http://127.0.0.1:17171/",
  namespace: "idp_store_a",
  retentionSeconds: 86_400,
};

function opening(
  reservation: NonNullable<ReturnType<ReturnType<typeof createMutationJournal>["reserve"]>>,
  execute: () => Promise<string>,
) {
  return reservation.track(createMutationPromise((_key) => execute(), reservation.idempotencyKey));
}

describe("persistent mutation journal", () => {
  it("persists a privacy-preserving identity before opening the transport", async () => {
    const storage = memoryStorage();
    const journal = createMutationJournal({ storage, scope: () => scope });
    const reservation = journal.reserve("providers.update", {
      provider: "secret-provider",
      apiKey: "must-not-enter-storage",
    });

    expect(reservation?.idempotencyKey).toBeTruthy();
    const encoded = JSON.stringify(storage.snapshot());
    expect(encoded).not.toContain("secret-provider");
    expect(encoded).not.toContain("must-not-enter-storage");
  });

  it("restores one unknown command after a process restart without merging a fresh twin", () => {
    const storage = memoryStorage();
    const firstJournal = createMutationJournal({ storage, scope: () => scope });
    const original = firstJournal.reserve("schedules.runNow", { id: "schedule_1" });

    // A new journal in the same process must not claim an active entry.
    const sameProcessStorage = memoryStorage(structuredClone(storage.snapshot()));
    const sameProcess = createMutationJournal({ storage: sameProcessStorage, scope: () => scope });
    expect(sameProcess.reserve("schedules.runNow", { id: "schedule_1" })?.idempotencyKey).not.toBe(
      original?.idempotencyKey,
    );

    // Simulate loading the same durable snapshot under the next process owner.
    const persisted = storage.snapshot() as { entries: Array<{ owner: string }> };
    for (const entry of persisted.entries) entry.owner = "previous-process";

    const restarted = createMutationJournal({ storage, scope: () => scope });
    const replay = restarted.reserve("schedules.runNow", { id: "schedule_1" });
    const freshTwin = restarted.reserve("schedules.runNow", { id: "schedule_1" });

    expect(replay?.idempotencyKey).toBe(original?.idempotencyKey);
    expect(freshTwin?.idempotencyKey).not.toBe(original?.idempotencyKey);
  });

  it("removes determinate outcomes and retains ambiguous ones for explicit retry", async () => {
    const storage = memoryStorage();
    const journal = createMutationJournal({ storage, scope: () => scope });
    const params = { sessionId: "ses_1" };
    const ambiguous = journal.reserve("goals.stop", params)!;
    const failure = new RpcTransportError("response lost");

    await expect(opening(ambiguous, () => Promise.reject(failure))).rejects.toBe(failure);
    expect(journal.reserve("goals.stop", params)?.idempotencyKey).toBe(ambiguous.idempotencyKey);

    const definitive = journal.reserve("goals.resume", params)!;
    const refusal = new RpcError({
      message: "paused goal missing",
      data: { type: "session_busy" },
    });
    await expect(opening(definitive, () => Promise.reject(refusal))).rejects.toBe(refusal);
    expect(journal.reserve("goals.resume", params)?.idempotencyKey).not.toBe(
      definitive.idempotencyKey,
    );

    const transportRefusal = journal.reserve("sessions.delete", params)!;
    const rejectedBeforeAdmission = new RpcTransportError("bad request", 400);
    await expect(
      opening(transportRefusal, () => Promise.reject(rejectedBeforeAdmission)),
    ).rejects.toBe(rejectedBeforeAdmission);
    expect(journal.reserve("sessions.delete", params)?.idempotencyKey).not.toBe(
      transportRefusal.idempotencyKey,
    );
  });

  it("removes a successful command and keeps the same key on MutationPromise.retry", async () => {
    const storage = memoryStorage();
    const journal = createMutationJournal({ storage, scope: () => scope });
    const reservation = journal.reserve("sessions.delete", { sessionId: "ses_1" })!;
    const execute = vi.fn().mockResolvedValue("deleted");
    const mutation = opening(reservation, execute);

    await expect(mutation).resolves.toBe("deleted");
    const retry = mutation.retry();
    expect(retry.idempotencyKey).toBe(reservation.idempotencyKey);
    await expect(retry).resolves.toBe("deleted");
  });

  it("fails closed across a replaced Runtime store, another endpoint, and expiry", () => {
    const storage = memoryStorage();
    let currentTime = 1_000;
    const firstJournal = createMutationJournal({
      storage,
      scope: () => scope,
      now: () => currentTime,
    });
    const original = firstJournal.reserve("sessions.create", { title: "same" })!;

    const replaced = createMutationJournal({
      storage,
      scope: () => ({ ...scope, namespace: "idp_store_b" }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(replaced.idempotencyKey).not.toBe(original.idempotencyKey);

    const otherEndpoint = createMutationJournal({
      storage,
      scope: () => ({ ...scope, endpoint: "http://127.0.0.1:27171" }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(otherEndpoint.idempotencyKey).not.toBe(replaced.idempotencyKey);

    currentTime += 86_400_000;
    const expired = createMutationJournal({
      storage,
      scope: () => ({ ...scope, endpoint: "http://127.0.0.1:27171" }),
      now: () => currentTime,
    }).reserve("sessions.create", { title: "same" })!;
    expect(expired.idempotencyKey).not.toBe(otherEndpoint.idempotencyKey);
  });

  it("ignores malformed persisted state and disables persistence before discovery", () => {
    const storage = memoryStorage({ version: 1, salt: "salt", entries: [{ nope: true }] });
    const undiscovered = createMutationJournal({ storage, scope: () => null });
    expect(undiscovered.reserve("sessions.delete", { sessionId: "ses_1" })).toBeUndefined();

    const discovered = createMutationJournal({ storage, scope: () => scope });
    expect(
      discovered.reserve("sessions.delete", { sessionId: "ses_1" })?.idempotencyKey,
    ).toBeTruthy();
  });
});
