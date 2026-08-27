import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  addAgentMemory,
  agentMemoryQuery,
  setAgentMemoryPinned,
} from "../application/agentMemoryConfig";
import { WORKSPACE_AGENT_MEMORY_KEY, type AgentMemoryEntry } from "../application/workspaceQueries";
import { installAgentMemoryGateway } from "./runtimeAgentMemoryGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
});

describe("runtimeAgentMemoryGateway", () => {
  it("maps returned add and update items into the workspace language", async () => {
    const item = {
      id: "memory_1",
      scope: "user",
      content: "Remember this",
      origin: "user",
      status: "active",
      pinned: false,
      createdAt: "2026-08-12T12:00:00Z",
      updatedAt: "2026-08-12T12:00:00Z",
    };
    const add = vi.fn().mockResolvedValue(item);
    const update = vi.fn().mockResolvedValue({
      ...item,
      pinned: true,
      updatedAt: "2026-08-12T12:00:01Z",
    });
    setContainer({
      client: () => ({ agentMemory: { add, update } }) as unknown as ScopeAppClient,
    });
    uninstall = installAgentMemoryGateway().dispose;

    await expect(addAgentMemory({ scope: "user", content: item.content })).resolves.toMatchObject({
      id: "memory_1",
      scope: "user",
      sessionId: "",
      day: "",
    });
    await expect(setAgentMemoryPinned("memory_1", true)).resolves.toBeUndefined();
    expect(update).toHaveBeenCalledWith({ id: "memory_1", pinned: true });
  });

  it("retires in-flight and queued commands before a successor gateway is installed", async () => {
    const query = agentMemoryQuery("user");
    const retiredUpdate = deferred<ReturnType<typeof memoryItem>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(
        memoryItem({ content: "successor", pinned: false, updatedAt: "2026-08-17T12:00:02Z" }),
      );
    setContainer({
      client: () => ({ agentMemory: { update: updateRetired } }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installAgentMemoryGateway();
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, query], [memoryEntry()]);

    const inFlight = setAgentMemoryPinned("memory_1", true);
    const queued = setAgentMemoryPinned("memory_1", false);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ agentMemory: { update: updateSuccessor } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installAgentMemoryGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData(
      [WORKSPACE_AGENT_MEMORY_KEY, query],
      [memoryEntry({ content: "successor", pinned: false })],
    );

    retiredUpdate.resolve(
      memoryItem({ content: "retired", pinned: true, updatedAt: "2026-08-17T12:00:01Z" }),
    );

    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query]),
    ).toEqual([memoryEntry({ content: "successor", pinned: false })]);

    const successorCommand = setAgentMemoryPinned("memory_1", false);
    retiredInstallation.replaceRuntimeGeneration();
    await expect(successorCommand).resolves.toBeUndefined();
    expect(updateSuccessor).toHaveBeenCalledOnce();
  });
});

function memoryItem(overrides: Record<string, unknown> = {}) {
  return {
    id: "memory_1",
    scope: "user" as const,
    content: "Remember this",
    origin: "user" as const,
    status: "active" as const,
    pinned: false,
    createdAt: "2026-08-17T12:00:00Z",
    updatedAt: "2026-08-17T12:00:00Z",
    ...overrides,
  };
}

function memoryEntry(overrides: Partial<AgentMemoryEntry> = {}): AgentMemoryEntry {
  return {
    ...memoryItem(),
    sessionId: "",
    day: "",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
