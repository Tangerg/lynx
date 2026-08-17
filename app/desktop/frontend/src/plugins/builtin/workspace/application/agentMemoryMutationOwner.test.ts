import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { AgentMemoryMutationOwner } from "./agentMemoryMutationOwner";
import type { AgentMemoryGateway } from "./ports/agentMemoryGateway";
import { WORKSPACE_AGENT_MEMORY_KEY, type AgentMemoryEntry } from "./workspaceQueries";

const QUERY = { scope: "user" } as const;
let owner: AgentMemoryMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
  vi.restoreAllMocks();
});

describe("AgentMemoryMutationOwner", () => {
  it("retires one Runtime command generation without draining it through the successor", async () => {
    const retired = deferred<AgentMemoryEntry>();
    const setPinned = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce(memory({ content: "successor", pinned: false }));
    owner = AgentMemoryMutationOwner.install({ setPinned } as unknown as AgentMemoryGateway);
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, QUERY], [memory()]);

    const inFlight = owner.setPinned("memory_1", true);
    const queued = owner.setPinned("memory_1", false);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(setPinned).toHaveBeenCalledOnce());

    owner.replaceRuntimeGeneration();
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "agent_memory_mutation_generation_retired",
    });
    expect(setPinned).toHaveBeenCalledOnce();

    retired.resolve(memory({ content: "retired", pinned: true }));
    await Promise.resolve();
    expect(queryClient.getQueryData([WORKSPACE_AGENT_MEMORY_KEY, QUERY])).toEqual([memory()]);

    await expect(owner.setPinned("memory_1", false)).resolves.toBeUndefined();
    expect(setPinned).toHaveBeenCalledTimes(2);
    expect(queryClient.getQueryData([WORKSPACE_AGENT_MEMORY_KEY, QUERY])).toEqual([
      memory({ content: "successor", pinned: false }),
    ]);
  });

  it("does not turn failed cache repair into an accepted mutation failure", async () => {
    const saved = memory({ pinned: true });
    owner = AgentMemoryMutationOwner.install({
      setPinned: vi.fn().mockResolvedValue(saved),
    } as unknown as AgentMemoryGateway);
    queryClient.setQueryData([WORKSPACE_AGENT_MEMORY_KEY, QUERY], [memory()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(owner.setPinned("memory_1", true)).resolves.toBeUndefined();
    expect(queryClient.getQueryData([WORKSPACE_AGENT_MEMORY_KEY, QUERY])).toEqual([saved]);
  });

  it("serializes one memory identity without blocking an unrelated item", async () => {
    const first = deferred<AgentMemoryEntry>();
    const setPinned = vi.fn((id: string, pinned: boolean) =>
      id === "memory_1"
        ? first.promise
        : Promise.resolve(memory({ id, content: "Independent", pinned })),
    );
    owner = AgentMemoryMutationOwner.install({ setPinned } as unknown as AgentMemoryGateway);
    queryClient.setQueryData(
      [WORKSPACE_AGENT_MEMORY_KEY, QUERY],
      [memory(), memory({ id: "memory_2", content: "Independent" })],
    );

    const blocked = owner.setPinned("memory_1", true);
    const independent = owner.setPinned("memory_2", true);

    await vi.waitFor(() => expect(setPinned).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toBeUndefined();
    first.resolve(memory({ pinned: true }));
    await expect(blocked).resolves.toBeUndefined();
  });
});

function memory(overrides: Partial<AgentMemoryEntry> = {}): AgentMemoryEntry {
  return {
    id: "memory_1",
    scope: "user",
    content: "Remember this",
    origin: "user",
    status: "active",
    pinned: false,
    sessionId: "",
    day: "",
    createdAt: "2026-08-17T12:00:00Z",
    updatedAt: "2026-08-17T12:00:00Z",
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
