import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentSessionMaterialRead,
  AgentSessionSnapshot,
  AgentRuntimeGateway,
} from "../ports/runtimeGateway";
import { configureAgentRuntimeGateway } from "../ports/runtimeGateway";
import { useAgentStore } from "../../adapters/agentStore";
import { installAgentStatePorts } from "../../adapters/agentStatePorts";
import {
  refreshAgentSessionProjection,
  revalidateAgentSessionProjection,
  synchronizeMountedAgentSession,
  synchronizeMountedAgentSessions,
} from "./refreshSessionProjection";

const SESSION_ID = "ses_refresh";

function snapshot(revision: number): AgentSessionSnapshot {
  return {
    items: [],
    runs: [],
    pendingInterruptSets: [],
    plan: {
      revision,
      steps: [],
    },
  };
}

function material(
  value: AgentSessionSnapshot,
  projectAssociatedSharedMaterial: AgentSessionMaterialRead["projectAssociatedSharedMaterial"] = (
    shared,
  ) => shared,
): AgentSessionMaterialRead {
  return { snapshot: value, projectAssociatedSharedMaterial };
}

function companion(label: string) {
  return vi.fn((shared: Record<string, unknown>) => ({ ...shared, companion: label }));
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
  it("lets a command await the mounted Session lifecycle owner", async () => {
    const synchronize = vi.fn().mockResolvedValue(true);
    useAgentStore.getState().setSynchronize(SESSION_ID, synchronize);

    await expect(synchronizeMountedAgentSession(SESSION_ID)).resolves.toBe(true);
    expect(synchronize).toHaveBeenCalledWith(undefined);
  });

  it("reports an unmounted Session instead of creating a second repair path", async () => {
    useAgentStore.getState().dropSession(SESSION_ID);

    await expect(synchronizeMountedAgentSession(SESSION_ID)).resolves.toBe(false);
  });

  it("keeps the old projection visible until the complete read commits", async () => {
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "old" });
    const visible = useAgentStore.getState().sessions[SESSION_ID]!.view;
    const read = deferred<AgentSessionMaterialRead>();
    const projectCompanion = companion("complete");
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toBe(visible);

    read.resolve(material(snapshot(1), projectCompanion));
    await expect(refreshing).resolves.toMatchObject({
      commandError: null,
      plan: { revision: 1 },
      shared: { companion: "complete" },
    });
    expect(projectCompanion).toHaveBeenCalledWith({});
  });

  it("discards an older read when a newer refresh starts", async () => {
    const older = deferred<AgentSessionMaterialRead>();
    const newer = deferred<AgentSessionMaterialRead>();
    const projectOlder = companion("older");
    const projectNewer = companion("newer");
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => older.promise)
      .mockImplementationOnce(() => newer.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);

    const olderRefresh = refreshAgentSessionProjection(SESSION_ID);
    const newerRefresh = refreshAgentSessionProjection(SESSION_ID);
    newer.resolve(material(snapshot(2), projectNewer));
    await expect(newerRefresh).resolves.toMatchObject({
      plan: { revision: 2 },
      shared: { companion: "newer" },
    });
    older.resolve(material(snapshot(1), projectOlder));
    await expect(olderRefresh).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view).toMatchObject({
      plan: { revision: 2 },
      shared: { companion: "newer" },
    });
    expect(projectNewer).toHaveBeenCalledOnce();
    expect(projectOlder).toHaveBeenCalledOnce();
  });

  it("discards a read that raced with a live projection write", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const projectCompanion = companion("raced");
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "live" });
    read.resolve(material(snapshot(1), projectCompanion));

    await expect(refreshing).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toEqual({
      code: "live",
    });
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.companion).toBeUndefined();
    expect(projectCompanion).toHaveBeenCalledOnce();
  });

  it("returns authoritative facts even when a newer local write rejects their commit", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const projectCompanion = companion("authoritative-only");
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const revalidating = revalidateAgentSessionProjection(SESSION_ID);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "live" });
    read.resolve(material(snapshot(4), projectCompanion));

    await expect(revalidating).resolves.toMatchObject({
      committed: false,
      authoritativeView: {
        plan: { revision: 4 },
        shared: { companion: "authoritative-only" },
      },
    });
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toEqual({
      code: "live",
    });
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.companion).toBeUndefined();
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
    const projectCompanion = companion("aborted");
    const controller = new AbortController();
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID, {
      signal: controller.signal,
    });
    controller.abort();
    await expect(refreshing).resolves.toBeNull();

    read.resolve(material(snapshot(9), projectCompanion));
    await Promise.resolve();
    const plan = useAgentStore.getState().sessions[SESSION_ID]!.view.plan;
    expect(plan?.revision).not.toBe(9);
    expect(projectCompanion).not.toHaveBeenCalled();
  });

  it("revokes an unsignaled snapshot at the Runtime connection boundary", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const projectCompanion = companion("retired");
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);

    const refreshing = refreshAgentSessionProjection(SESSION_ID);
    synchronizeMountedAgentSessions({ ownership: "retire-live" });
    read.resolve(material(snapshot(11), projectCompanion));

    await expect(refreshing).resolves.toBeNull();
    const plan = useAgentStore.getState().sessions[SESSION_ID]!.view.plan;
    expect(plan?.revision).not.toBe(11);
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.companion).toBeUndefined();
    expect(projectCompanion).toHaveBeenCalledOnce();
  });

  it("clears the old server projection before admitting its successor read", async () => {
    const predecessor = deferred<AgentSessionMaterialRead>();
    const successor = deferred<AgentSessionMaterialRead>();
    const loadSessionSnapshot = vi
      .fn()
      .mockImplementationOnce(() => predecessor.promise)
      .mockImplementationOnce(() => successor.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot,
    } as unknown as AgentRuntimeGateway);
    useAgentStore.getState().setCommandError(SESSION_ID, { code: "old-server" });

    const retiredRead = refreshAgentSessionProjection(SESSION_ID);
    synchronizeMountedAgentSessions({ ownership: "replace-server" });

    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.commandError).toBeNull();
    expect(loadSessionSnapshot).toHaveBeenCalledTimes(2);
    successor.resolve(material(snapshot(22)));
    await vi.waitFor(() =>
      expect(useAgentStore.getState().sessions[SESSION_ID]!.view.plan).toMatchObject({
        revision: 22,
      }),
    );

    predecessor.resolve(material(snapshot(11)));
    await expect(retiredRead).resolves.toBeNull();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.plan).toMatchObject({
      revision: 22,
    });
  });

  it("rejects an old port's snapshot and companion material after adapter replacement", async () => {
    const read = deferred<AgentSessionMaterialRead>();
    const projectCompanion = companion("retired-port");
    restoreRuntime = configureAgentRuntimeGateway({
      loadSessionSnapshot: vi.fn(() => read.promise),
    } as unknown as AgentRuntimeGateway);
    const disposeRetiredState = installAgentStatePorts();
    let disposeSuccessorState: (() => void) | undefined;
    try {
      const refreshing = refreshAgentSessionProjection(SESSION_ID);
      disposeSuccessorState = installAgentStatePorts();

      read.resolve(material(snapshot(10), projectCompanion));

      await expect(refreshing).resolves.toBeNull();
      const plan = useAgentStore.getState().sessions[SESSION_ID]!.view.plan;
      expect(plan?.revision).not.toBe(10);
      expect(useAgentStore.getState().sessions[SESSION_ID]!.view.shared.companion).toBeUndefined();
      expect(projectCompanion).toHaveBeenCalledOnce();
    } finally {
      disposeRetiredState();
      disposeSuccessorState?.();
      // This test replaces the process-local application ports. Leave the isolated
      // test worker with one live generation for any later test in the same module.
      installAgentStatePorts();
    }
  });
});
