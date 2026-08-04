import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentSessionSnapshot, AgentRuntimeGateway } from "../ports/runtimeGateway";
import { configureAgentRuntimeGateway } from "../ports/runtimeGateway";
import { useAgentStore } from "../../adapters/agentStore";
import { refreshAgentSessionProjection } from "./refreshSessionProjection";

const SESSION_ID = "ses_refresh";

function snapshot(revision: number): AgentSessionSnapshot {
  return {
    items: [],
    runs: [],
    pendingInterruptSets: [],
    state: {
      type: "plan",
      sessionId: SESSION_ID,
      revision,
      plan: [],
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

let restoreRuntime: (() => void) | undefined;

beforeEach(() => {
  useAgentStore.getState().dropSession(SESSION_ID);
  useAgentStore.getState().ensureSession(SESSION_ID);
});

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  useAgentStore.getState().dropSession(SESSION_ID);
});

describe("refreshAgentSessionProjection", () => {
  it("keeps the old projection visible until the complete read commits", async () => {
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "old" });
    const visible = useAgentStore.getState().sessions[SESSION_ID]!.view;
    const read = deferred<AgentSessionSnapshot>();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toBe(visible);

    read.resolve(snapshot(1));
    await expect(refreshing).resolves.toMatchObject({
      commandError: null,
      shared: { plan: { revision: 1 } },
    });
  });

  it("discards an older read when a newer refresh starts", async () => {
    const older = deferred<AgentSessionSnapshot>();
    const newer = deferred<AgentSessionSnapshot>();
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => older.promise)
      .mockImplementationOnce(() => newer.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);

    const olderRefresh = refreshAgentSessionProjection(SESSION_ID);
    const newerRefresh = refreshAgentSessionProjection(SESSION_ID);
    newer.resolve(snapshot(2));
    await expect(newerRefresh).resolves.toMatchObject({
      shared: { plan: { revision: 2 } },
    });
    older.resolve(snapshot(1));
    await expect(olderRefresh).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.plan).toMatchObject({
      revision: 2,
    });
  });

  it("discards a read that raced with a live projection write", async () => {
    const read = deferred<AgentSessionSnapshot>();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "live" });
    read.resolve(snapshot(1));

    await expect(refreshing).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toEqual({
      code: "live",
    });
  });

  it("leaves the prior projection intact when the durable read fails", async () => {
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "still-visible" });
    const visible = useAgentStore.getState().sessions[SESSION_ID]!.view;
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn().mockRejectedValue(new Error("offline")),
    } as unknown as AgentRuntimeGateway);

    await expect(refreshAgentSessionProjection(SESSION_ID)).rejects.toThrow("offline");
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toBe(visible);
  });
});
