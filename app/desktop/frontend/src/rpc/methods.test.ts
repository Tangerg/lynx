import { afterEach, describe, expect, it, vi } from "vitest";
import { createRpcClient, type RpcCallOptions, type RpcClient } from "./client";
import { RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import { asRunId, asSegmentId, asSessionId } from "./ids";
import { createMethods } from "./methods";
import {
  createMutationJournal,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  type MutationJournalStorage,
} from "./mutationJournal";
import type { Item, RunEvent, StreamEvent } from "./wire.generated";
import { RUN_EVENT_METHOD, RUNTIME_EVENT_METHOD } from "./stream";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import type { RpcMessage } from "./types";
import { JSONRPC_VERSION } from "./types";
import runRef from "./samples/runref.full.json";

function runEvent(runId: string, segmentId: string, eventId: string, event: StreamEvent): RunEvent {
  return { runId, segmentId, eventId, timestamp: "2026-06-03T00:00:00Z", event } as RunEvent;
}

function agentMessageItem(id: string, runId: string, status: Item["status"]): Item {
  return {
    id,
    runId,
    status,
    createdAt: "2026-06-03T00:00:00Z",
    type: "agentMessage",
    ...(status === "running" ? {} : { phase: "finalAnswer" as const }),
  } as Item;
}

afterEach(() => vi.useRealTimers());

describe("methods factory", () => {
  it("forwards the complete generated schedule update contract", async () => {
    const call = vi.fn().mockResolvedValue({ id: "schedule_1", revision: 3 });
    const methods = createMethods({ call } as unknown as RpcClient);

    await methods.schedules.update({
      id: "schedule_1",
      expectedRevision: 2,
      title: "Use the Runtime default",
      workspaceMode: "default",
    });

    expect(call).toHaveBeenCalledWith(
      "schedules.update",
      {
        id: "schedule_1",
        expectedRevision: 2,
        title: "Use the Runtime default",
        workspaceMode: "default",
      },
      expect.objectContaining({ idempotencyKey: expect.any(String) }),
    );
  });

  it("binds an immutable workspace identity into every nested resource call", async () => {
    const call = vi.fn().mockResolvedValue({ data: [] });
    const methods = createMethods({ call } as unknown as RpcClient);
    const ref = { path: "/repo" };
    const workspace = methods.workspace(ref);

    ref.path = "/retargeted";
    await workspace.files.list({ path: "src" });
    await workspace.agentMemory.list();

    expect(workspace.ref).toEqual({ path: "/repo" });
    expect(call).toHaveBeenCalledWith(
      "workspace.files.list",
      { path: "src", workspace: { path: "/repo" } },
      undefined,
    );
    expect(call).toHaveBeenCalledWith(
      "agentMemory.list",
      { scope: "project", workspace: { path: "/repo" } },
      undefined,
    );
  });

  it("resolves and caches the default workspace when opening an omitted ref", async () => {
    const call = vi.fn(async (method: string) => {
      if (method === "workspaces.resolve") {
        return {
          ref: { path: "/default" },
          projectRoot: "/default",
          availability: "available",
        };
      }
      return { data: [] };
    });
    const methods = createMethods({ call } as unknown as RpcClient);

    const first = await methods.workspaces.open();
    const second = await methods.workspaces.open();
    await second.changes.list();

    expect(first.ref).toEqual({ path: "/default" });
    expect(call.mock.calls.filter(([method]) => method === "workspaces.resolve")).toHaveLength(1);
    expect(call).toHaveBeenLastCalledWith(
      "workspace.changes.list",
      { workspace: { path: "/default" } },
      undefined,
    );
  });

  it("forwards cancellation when resolving a workspace", async () => {
    const call = vi.fn().mockResolvedValue({
      ref: { path: "/repo" },
      projectRoot: "/repo",
      availability: "available",
    });
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.workspaces.resolve({ path: "/repo" }, signal);

    expect(call).toHaveBeenCalledWith("workspaces.resolve", { ref: { path: "/repo" } }, { signal });
  });

  it("forwards cancellation when listing workspaces", async () => {
    const call = vi.fn().mockResolvedValue({ data: [] });
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.workspaces.list(signal);

    expect(call).toHaveBeenCalledWith("workspaces.list", {}, { signal });
  });

  it("forwards cancellation through a bound workspace change read", async () => {
    const call = vi.fn().mockResolvedValue({ data: [] });
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.workspace({ path: "/repo" }).changes.list(signal);

    expect(call).toHaveBeenCalledWith(
      "workspace.changes.list",
      { workspace: { path: "/repo" } },
      { signal },
    );
  });

  it("forwards cancellation through bound workspace file pagination", async () => {
    const call = vi.fn().mockResolvedValue({ data: [] });
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.workspace({ path: "/repo" }).files.list({ path: "src" }, signal);

    expect(call).toHaveBeenCalledWith(
      "workspace.files.list",
      { path: "src", workspace: { path: "/repo" } },
      { signal },
    );
  });

  it("forwards cancellation when reading a session", async () => {
    const call = vi.fn().mockResolvedValue({ id: "ses_1" });
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.sessions.get(asSessionId("ses_1"), signal);

    expect(call).toHaveBeenCalledWith("sessions.get", { sessionId: "ses_1" }, { signal });
  });

  it.each([
    { includeDescendants: false, expected: {} },
    { includeDescendants: true, expected: { includeDescendants: true } },
  ])(
    "forwards cancellation through a coherent session snapshot with descendants=$includeDescendants",
    async ({ includeDescendants, expected }) => {
      const call = vi.fn().mockResolvedValue({ items: [], runs: [], interrupts: [] });
      const methods = createMethods({ call } as unknown as RpcClient);
      const signal = new AbortController().signal;

      await methods.sessions.snapshot(asSessionId("ses_1"), includeDescendants, signal);

      expect(call).toHaveBeenCalledWith(
        "sessions.snapshot",
        { sessionId: "ses_1", ...expected },
        { signal },
      );
    },
  );

  it("forwards cancellation when reading a Goal", async () => {
    const call = vi.fn().mockResolvedValue(null);
    const methods = createMethods({ call } as unknown as RpcClient);
    const signal = new AbortController().signal;

    await methods.goals.get(asSessionId("ses_1"), signal);

    expect(call).toHaveBeenCalledWith("goals.get", { sessionId: "ses_1" }, { signal });
  });

  it("forwards Goal update and clear through the generated mutation contract", async () => {
    const call = vi.fn().mockResolvedValue({
      sessionId: "ses_1",
      objective: "Ship beta",
      status: "active",
      budget: {},
      used: { runs: 0, costUsd: 0, steps: 0 },
    });
    const methods = createMethods({ call } as unknown as RpcClient);

    await methods.goals.update({ sessionId: asSessionId("ses_1"), objective: "Ship beta" });
    await methods.goals.clear(asSessionId("ses_1"));

    expect(call).toHaveBeenCalledWith(
      "goals.update",
      { sessionId: "ses_1", objective: "Ship beta" },
      expect.objectContaining({ idempotencyKey: expect.any(String) }),
    );
    expect(call).toHaveBeenCalledWith(
      "goals.clear",
      { sessionId: "ses_1" },
      expect.objectContaining({ idempotencyKey: expect.any(String) }),
    );
  });

  it("retries default workspace resolution after a transient failure", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockResolvedValueOnce({
        ref: { path: "/recovered" },
        projectRoot: "/recovered",
        availability: "available",
      });
    const methods = createMethods({ call } as unknown as RpcClient);

    await expect(methods.workspaces.open()).rejects.toBeInstanceOf(RpcTransportError);
    await expect(methods.workspaces.open()).resolves.toMatchObject({
      ref: { path: "/recovered" },
    });
    expect(call).toHaveBeenCalledTimes(2);
  });

  it("automatically replays an ambiguously failed mutation with the same key", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockResolvedValueOnce({ sessionId: "ses_1", runId: "run_1" })
      .mockResolvedValueOnce({ sessionId: "ses_2", runId: "run_2" });
    const client = { call } as unknown as RpcClient;
    const methods = createMethods(client);

    const attempt = methods.schedules.runNow("schedule_1");
    await expect(attempt).resolves.toMatchObject({
      runId: "run_1",
    });
    await expect(methods.schedules.runNow("schedule_1")).resolves.toMatchObject({
      runId: "run_2",
    });

    const keys = call.mock.calls.map((args) => args[2]?.idempotencyKey as string);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[1]);
  });

  it("does not replay an old mutation identity into a replaced Runtime store", async () => {
    const values = new Map<string, unknown>();
    const storage: MutationJournalStorage = {
      get: (key) => {
        const value = values.get(key);
        return value === undefined ? undefined : structuredClone(value);
      },
      set: (key, value) => values.set(key, structuredClone(value)),
      remove: (key) => void values.delete(key),
      keys: () => [...values.keys()],
    };
    let namespace = "idp_runtime_store_a";
    const persistentJournal = createMutationJournal({
      storage,
      scope: () => ({
        namespace,
        retentionSeconds: 86_400,
      }),
    });
    const call = vi
      .fn()
      .mockImplementationOnce(async () => {
        namespace = "idp_runtime_store_b";
        throw new RpcTransportError("old Runtime response lost");
      })
      .mockResolvedValue({ sessionId: "ses_2", runId: "run_2" });
    const methods = createMethods({ call } as unknown as RpcClient, {
      mutationJournal: persistentJournal,
    });

    const retired = methods.schedules.runNow("schedule_1");
    await expect(retired).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    expect(call).toHaveBeenCalledOnce();
    expect(call.mock.calls[0]?.[2]?.idempotencyNamespace).toBe("idp_runtime_store_a");

    await expect(retired.retry()).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    expect(call).toHaveBeenCalledOnce();

    const replacement = methods.schedules.runNow("schedule_1");
    await expect(replacement).resolves.toMatchObject({ runId: "run_2" });
    expect(call).toHaveBeenCalledTimes(2);
    expect(call.mock.calls[1]?.[2]?.idempotencyKey).not.toBe(retired.idempotencyKey);
    expect(call.mock.calls[1]?.[2]?.idempotencyNamespace).toBe("idp_runtime_store_b");
    persistentJournal.dispose();
  });

  it("retains an unknown mutation while the Runtime store identity is unavailable", async () => {
    const values = new Map<string, unknown>();
    const storage: MutationJournalStorage = {
      get: (key) => values.get(key),
      set: (key, value) => values.set(key, structuredClone(value)),
      remove: (key) => void values.delete(key),
      keys: () => [...values.keys()],
    };
    let scopeAvailable = true;
    const persistentJournal = createMutationJournal({
      storage,
      scope: () =>
        scopeAvailable
          ? {
              namespace: "idp_runtime_store",
              retentionSeconds: 86_400,
            }
          : null,
    });
    const call = vi
      .fn()
      .mockImplementationOnce(async () => {
        scopeAvailable = false;
        throw new RpcTransportError("Runtime discovery withdrawn");
      })
      .mockResolvedValue({ sessionId: "ses_1", runId: "run_1" });
    const methods = createMethods({ call } as unknown as RpcClient, {
      mutationJournal: persistentJournal,
    });

    const pending = methods.schedules.runNow("schedule_1");
    await expect(pending).rejects.toBeInstanceOf(MutationJournalScopeUnavailableError);
    expect(call).toHaveBeenCalledOnce();
    const originalKey = pending.idempotencyKey;
    expect(
      [...values.values()].some(
        (value) => (value as { idempotencyKey?: string }).idempotencyKey === originalKey,
      ),
    ).toBe(true);

    scopeAvailable = true;
    const replay = pending.retry();
    await expect(replay).resolves.toMatchObject({ runId: "run_1" });
    expect(call).toHaveBeenCalledTimes(2);
    expect(call.mock.calls[1]?.[2]?.idempotencyKey).toBe(originalKey);
    expect(call.mock.calls[1]?.[2]?.idempotencyNamespace).toBe("idp_runtime_store");
    persistentJournal.dispose();
  });

  it("restores a persisted unary mutation key after a Desktop process loss", async () => {
    const values = new Map<string, unknown>();
    const storage: MutationJournalStorage = {
      get: (key) => {
        const value = values.get(key);
        return value === undefined ? undefined : structuredClone(value);
      },
      set: (key, value) => {
        values.set(key, structuredClone(value));
      },
      remove: (key) => void values.delete(key),
      keys: () => [...values.keys()],
    };
    const scope = () => ({
      namespace: "idp_runtime_store",
      retentionSeconds: 86_400,
    });
    let settleFirst!: (value: { sessionId: string; runId: string }) => void;
    const firstCall = vi.fn(
      (_method: string, _params: unknown, _options?: RpcCallOptions) =>
        new Promise<{ sessionId: string; runId: string }>((resolve) => {
          settleFirst = resolve;
        }),
    );
    const firstJournal = createMutationJournal({ storage, scope });
    const firstMethods = createMethods({ call: firstCall } as unknown as RpcClient, {
      mutationJournal: firstJournal,
    });

    const first = firstMethods.schedules.runNow("schedule_1");
    await vi.waitFor(() => expect(firstCall).toHaveBeenCalledOnce());
    const firstRequest = firstCall.mock.calls[0];
    expect(firstRequest).toBeDefined();
    const firstKey = (firstRequest?.[2] as RpcCallOptions | undefined)?.idempotencyKey;
    expect(firstKey).toBeTruthy();

    const replayCall = vi.fn(
      async (_method: string, _params: unknown, _options?: RpcCallOptions) => ({
        sessionId: "ses_1",
        runId: "run_1",
      }),
    );
    const restartedJournal = createMutationJournal({ storage, scope });
    const restartedMethods = createMethods({ call: replayCall } as unknown as RpcClient, {
      mutationJournal: restartedJournal,
    });
    await expect(restartedMethods.schedules.runNow("schedule_1")).resolves.toMatchObject({
      runId: "run_1",
    });

    expect(replayCall.mock.calls[0]?.[2]?.idempotencyKey).toBe(firstKey);
    settleFirst({ sessionId: "ses_1", runId: "run_1" });
    await expect(first).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    firstJournal.dispose();
    restartedJournal.dispose();
  });

  it("replays the same mutation identity after a client hot-swap", async () => {
    const values = new Map<string, unknown>();
    const storage: MutationJournalStorage = {
      get: (key) => {
        const value = values.get(key);
        return value === undefined ? undefined : structuredClone(value);
      },
      set: (key, value) => values.set(key, structuredClone(value)),
      remove: (key) => void values.delete(key),
      keys: () => [...values.keys()],
    };
    const scope = () => ({
      namespace: "idp_runtime_store",
      retentionSeconds: 86_400,
    });
    let settleRetired!: (value: { sessionId: string; runId: string }) => void;
    const retiredCall = vi.fn(
      (_method: string, _params: unknown, _options?: RpcCallOptions) =>
        new Promise<{ sessionId: string; runId: string }>((resolve) => {
          settleRetired = resolve;
        }),
    );
    const retiredJournal = createMutationJournal({ storage, scope });
    const retiredMethods = createMethods({ call: retiredCall } as unknown as RpcClient, {
      mutationJournal: retiredJournal,
    });
    const retired = retiredMethods.schedules.runNow("schedule_1");
    await vi.waitFor(() => expect(retiredCall).toHaveBeenCalledOnce());

    let settleReplacement!: (value: { sessionId: string; runId: string }) => void;
    const replacementCall = vi.fn(
      (_method: string, _params: unknown, _options?: RpcCallOptions) =>
        new Promise<{ sessionId: string; runId: string }>((resolve) => {
          settleReplacement = resolve;
        }),
    );
    const replacementJournal = createMutationJournal({ storage, scope });
    const replacementMethods = createMethods({ call: replacementCall } as unknown as RpcClient, {
      mutationJournal: replacementJournal,
    });
    const replay = replacementMethods.schedules.runNow("schedule_1");
    await vi.waitFor(() => expect(replacementCall).toHaveBeenCalledOnce());

    const retiredKey = retiredCall.mock.calls[0]?.[2]?.idempotencyKey;
    expect(replacementCall.mock.calls[0]?.[2]?.idempotencyKey).toBe(retiredKey);
    settleRetired({ sessionId: "ses_1", runId: "run_1" });
    await expect(retired).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    expect([...values.keys()].some((key) => key.startsWith("entry:"))).toBe(true);
    retiredJournal.dispose();

    settleReplacement({ sessionId: "ses_1", runId: "run_1" });
    await expect(replay).resolves.toMatchObject({ runId: "run_1" });
    expect([...values.keys()].some((key) => key.startsWith("entry:"))).toBe(false);
    replacementJournal.dispose();
  });

  it("does not open the transport until mutation identity persistence recovers", async () => {
    let failWrites = true;
    const values = new Map<string, unknown>();
    const storage: MutationJournalStorage = {
      get: (key) => values.get(key),
      set: (key, value) => {
        if (failWrites) throw new Error("quota exceeded");
        values.set(key, structuredClone(value));
      },
      remove: (key) => void values.delete(key),
      keys: () => [...values.keys()],
    };
    const persistentJournal = createMutationJournal({
      storage,
      scope: () => ({
        namespace: "idp_runtime_store",
        retentionSeconds: 86_400,
      }),
    });
    const call = vi.fn().mockResolvedValue({ sessionId: "ses_1", runId: "run_1" });
    const methods = createMethods({ call } as unknown as RpcClient, {
      mutationJournal: persistentJournal,
    });

    const failed = methods.schedules.runNow("schedule_1");
    await expect(failed).rejects.toThrow("not persisted");
    expect(call).not.toHaveBeenCalled();

    failWrites = false;
    const recovered = failed.retry();
    expect(recovered.idempotencyKey).toBe(failed.idempotencyKey);
    await expect(recovered).resolves.toMatchObject({ runId: "run_1" });
    expect(call).toHaveBeenCalledOnce();
    persistentJournal.dispose();
  });

  it("opens a fresh event stream when replaying an ambiguous run opening", async () => {
    const unsubscribers = [vi.fn(), vi.fn()];
    const subscribe = vi
      .fn()
      .mockReturnValueOnce(unsubscribers[0])
      .mockReturnValueOnce(unsubscribers[1]);
    const onStreamEnd = vi
      .fn()
      .mockReturnValueOnce(unsubscribers[0])
      .mockReturnValueOnce(unsubscribers[1]);
    const call = vi
      .fn()
      .mockImplementationOnce(async (_method, _params, options: RpcCallOptions) => {
        options.onRequestRpcId?.("request_1");
        throw new RpcTransportError("opening response lost");
      })
      .mockImplementationOnce(async (_method, _params, options: RpcCallOptions) => {
        options.onRequestRpcId?.("request_2");
        return { runId: "run_1", segmentId: "seg_1", userItemId: "item_user_1" };
      });
    const methods = createMethods({ call, subscribe, onStreamEnd } as unknown as RpcClient);

    const opened = await methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hello" }],
    });

    expect(opened.result).toMatchObject({ runId: "run_1", segmentId: "seg_1" });
    expect(call).toHaveBeenCalledTimes(2);
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(onStreamEnd).toHaveBeenCalledTimes(2);
    expect(unsubscribers[0]).toHaveBeenCalledTimes(2);
    const first = call.mock.calls[0]?.[2] as RpcCallOptions;
    const second = call.mock.calls[1]?.[2] as RpcCallOptions;
    expect(second.idempotencyKey).toBe(first.idempotencyKey);
    expect(second.signal).not.toBe(first.signal);
    expect(first.signal?.aborted).toBe(true);
    expect(second.signal?.aborted).toBe(false);

    await opened.events[Symbol.asyncIterator]().return?.();
    expect(unsubscribers[1]).toHaveBeenCalledTimes(2);
  });

  it("gives concurrent mutations with identical params distinct invocation keys", async () => {
    const call = vi
      .fn()
      .mockResolvedValueOnce({ sessionId: "ses_1", runId: "run_1" })
      .mockResolvedValueOnce({ sessionId: "ses_2", runId: "run_2" });
    const methods = createMethods({ call } as unknown as RpcClient);

    const first = methods.schedules.runNow("schedule_1");
    const second = methods.schedules.runNow("schedule_1");
    await Promise.all([first, second]);

    expect(first.idempotencyKey).toBeTruthy();
    expect(second.idempotencyKey).toBeTruthy();
    expect(first.idempotencyKey).not.toBe(second.idempotencyKey);
    expect(call.mock.calls[0]?.[2]?.idempotencyKey).toBe(first.idempotencyKey);
    expect(call.mock.calls[1]?.[2]?.idempotencyKey).toBe(second.idempotencyKey);
  });

  it("honors the server backoff while the original mutation is in progress", async () => {
    vi.useFakeTimers();
    const call = vi
      .fn()
      .mockRejectedValueOnce(
        new RpcError({
          code: -32021,
          message: "idempotency_in_progress",
          data: { type: "idempotency_in_progress", retryAfterSeconds: 1 },
        }),
      )
      .mockResolvedValueOnce({ id: "session_1" });
    const client = { call } as unknown as RpcClient;
    const methods = createMethods(client);

    const attempt = methods.sessions.create({ title: "same", workspace: { path: "/repo" } });
    await vi.advanceTimersByTimeAsync(999);
    expect(call).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    await expect(attempt).resolves.toMatchObject({ id: "session_1" });

    const first = call.mock.calls[0]?.[2] as RpcCallOptions | undefined;
    const second = call.mock.calls[1]?.[2] as RpcCallOptions | undefined;
    expect(second?.idempotencyKey).toBe(first?.idempotencyKey);
    vi.useRealTimers();
  });

  it("sessions.list sends sessions.list with optional query and returns a Page", async () => {
    const t = createMemoryTransport();
    const send = vi.spyOn(t, "send");
    const client = createRpcClient(t);
    const methods = createMethods(client);
    const signal = new AbortController().signal;

    const promise = methods.sessions.list({ limit: 10 }, signal);
    const req = await waitForRequest(t, "sessions.list");
    expect(req.method).toBe("sessions.list");
    expect(req.params).toEqual({ limit: 10 });
    expect(send.mock.calls[0]?.[1]).toBe(signal);

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: { data: [] } } as RpcMessage);
    await expect(promise).resolves.toEqual({ data: [] });
    await client.close();
  });

  it("runs.list forwards session filtering, status filtering and pagination", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.runs.list({
      sessionId: asSessionId("ses_1"),
      statuses: ["running", "waiting"],
      cursor: "next",
      limit: 25,
    });
    const req = await waitForRequest(t, "runs.list");
    expect(req.params).toEqual({
      sessionId: "ses_1",
      statuses: ["running", "waiting"],
      cursor: "next",
      limit: 25,
    });

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: { data: [] } } as RpcMessage);
    await expect(promise).resolves.toEqual({ data: [] });
    await client.close();
  });

  it("derives continuation calls from the paged method policy", async () => {
    const controller = new AbortController();
    const call = vi
      .fn()
      .mockResolvedValueOnce({ data: [{ id: "run_1" }], nextCursor: "cursor_2" })
      .mockResolvedValueOnce({ data: [{ id: "run_2" }] });
    const methods = createMethods({ call } as unknown as RpcClient);

    const runs = await methods.runs
      .list(
        {
          sessionId: asSessionId("ses_1"),
          statuses: ["finished"],
          limit: 25,
        },
        controller.signal,
      )
      .autoPagingToArray();

    expect(runs.map((run) => run.id)).toEqual(["run_1", "run_2"]);
    expect(call.mock.calls).toEqual([
      [
        "runs.list",
        { sessionId: "ses_1", statuses: ["finished"], limit: 25 },
        { signal: controller.signal },
      ],
      [
        "runs.list",
        {
          sessionId: "ses_1",
          statuses: ["finished"],
          limit: 25,
          cursor: "cursor_2",
        },
        { signal: controller.signal },
      ],
    ]);
  });

  it("items.list forwards the scope and the page direction", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.items.list({
      scope: { type: "run", runId: asRunId("run_1") },
      order: "desc",
      limit: 50,
    });
    const req = await waitForRequest(t, "items.list");
    expect(req.params).toEqual({
      scope: { type: "run", runId: "run_1" },
      order: "desc",
      limit: 50,
    });

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { data: [], runs: [] },
    } as RpcMessage);
    await expect(promise).resolves.toEqual({ data: [], runs: [] });
    await client.close();
  });

  it("runs.get asks by run id alone", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.runs.get(asRunId("run_1"));
    const req = await waitForRequest(t, "runs.get");
    expect(req.params).toEqual({ runId: "run_1" });

    const result = { ...runRef, id: "run_1" };
    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result } as RpcMessage);
    await expect(promise).resolves.toEqual(result);
    await client.close();
  });

  it("runs.steer keeps structured content blocks on the wire", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);
    const input = [
      { type: "text" as const, text: "compare this" },
      { type: "image" as const, mime: "image/png", data: "aW1hZ2U=" },
    ];

    const promise = methods.runs.steer(asRunId("run_1"), asSegmentId("seg_1"), input);
    const req = await waitForRequest(t, "runs.steer");
    expect(req.params).toEqual({
      runId: "run_1",
      expectedSegmentId: "seg_1",
      input,
    });

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: {} } as RpcMessage);
    await expect(promise).resolves.toBeUndefined();
    await client.close();
  });

  it("runs.start returns a streaming result that ends on the root segment's segment.finished", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(t, "runs.start");
    expect(req.params).toMatchObject({ sessionId: "ses_1" });

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "run_1", segmentId: "seg_1", userItemId: "item_user_1" },
    } as RpcMessage);
    const { result, events } = await startPromise;
    expect(result.runId).toBe("run_1");

    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUN_EVENT_METHOD,
        params: runEvent("run_1", "seg_1", "evt_1", {
          type: "item.started",
          item: agentMessageItem("item_1", "run_1", "running"),
        }),
      },
      undefined,
      req.id,
    );
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUN_EVENT_METHOD,
        params: runEvent("run_1", "seg_1", "evt_2", {
          type: "segment.finished",
          outcome: { type: "completed" },
          metrics: { steps: 0, activeDurationMillis: 0 },
        }),
      },
      undefined,
      req.id,
    );

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.started", "segment.finished"]);
    await client.close();
  });

  it("ignores events for foreign segments", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(t, "runs.start");
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "run_1", segmentId: "seg_1", userItemId: "item_user_1" },
    } as RpcMessage);
    const { events } = await startPromise;

    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUN_EVENT_METHOD,
        params: runEvent("run_OTHER", "seg_OTHER", "evt_x", {
          type: "item.started",
          item: agentMessageItem("item_x", "run_OTHER", "running"),
        }),
      },
      undefined,
      req.id,
    );
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUN_EVENT_METHOD,
        params: runEvent("run_1", "seg_1", "evt_1", {
          type: "item.completed",
          item: agentMessageItem("item_1", "run_1", "completed"),
        }),
      },
      undefined,
      req.id,
    );
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUN_EVENT_METHOD,
        params: runEvent("run_1", "seg_1", "evt_2", {
          type: "segment.finished",
          outcome: { type: "completed" },
          metrics: { steps: 0, activeDurationMillis: 0 },
        }),
      },
      undefined,
      req.id,
    );

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.completed", "segment.finished"]);
    await client.close();
  });

  it("keeps concurrent runtime subscriptions isolated by their response stream", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const sessionsPromise = methods.runtimeEvents.subscribe({ topics: ["sessions.changed"] });
    const sessionsRequest = await waitForRequest(t, "runtime.subscribe");
    t.inject({ jsonrpc: JSONRPC_VERSION, id: sessionsRequest.id, result: {} } as RpcMessage);
    const sessions = await sessionsPromise;

    const schedulesPromise = methods.runtimeEvents.subscribe({ topics: ["schedules.changed"] });
    await Promise.resolve();
    const schedulesRequest = t
      .outbox()
      .find(
        (request) => request.method === "runtime.subscribe" && request.id !== sessionsRequest.id,
      );
    expect(schedulesRequest).toBeDefined();
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: schedulesRequest!.id,
      result: {},
    } as RpcMessage);
    const schedules = await schedulesPromise;

    const sessionsIterator = sessions.events[Symbol.asyncIterator]();
    const schedulesIterator = schedules.events[Symbol.asyncIterator]();
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUNTIME_EVENT_METHOD,
        params: {
          event: { type: "sessions.changed", sequence: 1, sessionIds: ["session_1"] },
        },
      },
      undefined,
      sessionsRequest.id,
    );
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUNTIME_EVENT_METHOD,
        params: {
          event: { type: "schedules.changed", sequence: 1, scheduleIds: ["schedule_1"] },
        },
      },
      undefined,
      schedulesRequest!.id,
    );

    await expect(sessionsIterator.next()).resolves.toMatchObject({
      value: { type: "sessions.changed" },
    });
    await expect(schedulesIterator.next()).resolves.toMatchObject({
      value: { type: "schedules.changed" },
    });

    t.endStream("runtime.subscribe", sessionsRequest.id);
    await expect(sessionsIterator.next()).resolves.toMatchObject({ done: true });
    t.endStream("runtime.subscribe", schedulesRequest!.id);
    await expect(schedulesIterator.next()).resolves.toMatchObject({ done: true });
    await client.close();
  });

  it("scopes an invalid notification to the response stream that carried it", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const sessionsPromise = methods.runtimeEvents.subscribe({ topics: ["sessions.changed"] });
    const sessionsRequest = await waitForRequest(t, "runtime.subscribe");
    t.inject({ jsonrpc: JSONRPC_VERSION, id: sessionsRequest.id, result: {} } as RpcMessage);
    const sessions = await sessionsPromise;

    const schedulesPromise = methods.runtimeEvents.subscribe({ topics: ["schedules.changed"] });
    await Promise.resolve();
    const schedulesRequest = t
      .outbox()
      .find(
        (request) => request.method === "runtime.subscribe" && request.id !== sessionsRequest.id,
      );
    expect(schedulesRequest).toBeDefined();
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: schedulesRequest!.id,
      result: {},
    } as RpcMessage);
    const schedules = await schedulesPromise;

    const sessionsIterator = sessions.events[Symbol.asyncIterator]();
    const schedulesIterator = schedules.events[Symbol.asyncIterator]();
    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUNTIME_EVENT_METHOD,
        params: { event: { type: "sessions.changed", sequence: 0 } },
      },
      undefined,
      sessionsRequest.id,
    );
    await expect(sessionsIterator.next()).rejects.toBeInstanceOf(RpcProtocolError);

    t.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: RUNTIME_EVENT_METHOD,
        params: { event: { type: "schedules.changed", sequence: 1 } },
      },
      undefined,
      schedulesRequest!.id,
    );
    await expect(schedulesIterator.next()).resolves.toMatchObject({
      value: { type: "schedules.changed" },
    });

    t.endStream("runtime.subscribe", schedulesRequest!.id);
    await expect(schedulesIterator.next()).resolves.toMatchObject({ done: true });
    await client.close();
  });
});
