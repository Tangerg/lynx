import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentSessionMaterialRead,
  AgentSessionSnapshot,
  AgentRuntimeGateway,
} from "../ports/runtimeGateway";
import { configureAgentRuntimeGateway } from "../ports/runtimeGateway";
import { useAgentStore } from "../../adapters/agentStore";
import {
  refreshAgentSessionProjection,
  revalidateAgentSessionProjection,
} from "./refreshSessionProjection";

const SESSION_ID = "ses_refresh";

function snapshot(revision: number): AgentSessionSnapshot {
  return {
    items: [],
    runs: [],
    pendingInterruptSets: [],
    state: {
      type: "plan",
      revision,
      plan: [],
    },
  };
}

function material(
  value: AgentSessionSnapshot,
  commitAssociatedReadModels = vi.fn(),
): AgentSessionMaterialRead {
  return { snapshot: value, commitAssociatedReadModels };
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
    const read = deferred<AgentSessionMaterialRead>();
    const commitAssociatedReadModels = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toBe(visible);

    read.resolve(material(snapshot(1), commitAssociatedReadModels));
    await expect(refreshing).resolves.toMatchObject({
      commandError: null,
      shared: { plan: { revision: 1 } },
    });
    expect(commitAssociatedReadModels).toHaveBeenCalledOnce();
  });

  it("discards an older read when a newer refresh starts", async () => {
    const older = deferred<AgentSessionMaterialRead>();
    const newer = deferred<AgentSessionMaterialRead>();
    const commitOlder = vi.fn();
    const commitNewer = vi.fn();
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => older.promise)
      .mockImplementationOnce(() => newer.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);

    const olderRefresh = refreshAgentSessionProjection(SESSION_ID);
    const newerRefresh = refreshAgentSessionProjection(SESSION_ID);
    newer.resolve(material(snapshot(2), commitNewer));
    await expect(newerRefresh).resolves.toMatchObject({
      shared: { plan: { revision: 2 } },
    });
    older.resolve(material(snapshot(1), commitOlder));
    await expect(olderRefresh).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.plan).toMatchObject({
      revision: 2,
    });
    expect(commitNewer).toHaveBeenCalledOnce();
    expect(commitOlder).not.toHaveBeenCalled();
  });

  it("discards a read that raced with a live projection write", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const commitAssociatedReadModels = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "live" });
    read.resolve(material(snapshot(1), commitAssociatedReadModels));

    await expect(refreshing).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toEqual({
      code: "live",
    });
    expect(commitAssociatedReadModels).not.toHaveBeenCalled();
  });

  it("returns authoritative facts even when a newer local write rejects their commit", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const commitAssociatedReadModels = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const revalidating = revalidateAgentSessionProjection(SESSION_ID);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "live" });
    read.resolve(material(snapshot(4), commitAssociatedReadModels));

    await expect(revalidating).resolves.toMatchObject({
      committed: false,
      authoritativeView: { shared: { plan: { revision: 4 } } },
    });
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toEqual({
      code: "live",
    });
    expect(commitAssociatedReadModels).not.toHaveBeenCalled();
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

  it("treats an authoritatively missing session as no applicable projection", async () => {
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "still-visible" });
    const visible = useAgentStore.getState().sessions[SESSION_ID]!.view;
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn().mockResolvedValue(null),
    } as unknown as AgentRuntimeGateway);

    await expect(refreshAgentSessionProjection(SESSION_ID)).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toBe(visible);
  });

  it("retires a non-cooperative snapshot read without allowing a late commit", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const commitAssociatedReadModels = vi.fn();
    const controller = new AbortController();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID, {
      signal: controller.signal,
    });
    controller.abort();
    await expect(refreshing).resolves.toBeNull();

    read.resolve(material(snapshot(9), commitAssociatedReadModels));
    await Promise.resolve();
    const plan = useAgentStore.getState().sessions[SESSION_ID]!.view.shared.plan as
      { revision?: number } | null | undefined;
    expect(plan?.revision).not.toBe(9);
    expect(commitAssociatedReadModels).not.toHaveBeenCalled();
  });
});
