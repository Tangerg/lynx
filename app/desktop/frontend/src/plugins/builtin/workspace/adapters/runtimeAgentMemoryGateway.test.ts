import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { agentMemoryGateway } from "../application/ports/agentMemoryGateway";
import { installAgentMemoryGateway } from "./runtimeAgentMemoryGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
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
      client: () => ({ agentMemory: { add, update } }) as unknown as LyraClient,
    });
    uninstall = installAgentMemoryGateway();

    await expect(
      agentMemoryGateway().add({ scope: "user", content: item.content }),
    ).resolves.toMatchObject({ id: "memory_1", scope: "user", sessionId: "", day: "" });
    await expect(agentMemoryGateway().setPinned("memory_1", true)).resolves.toMatchObject({
      id: "memory_1",
      pinned: true,
      updatedAt: "2026-08-12T12:00:01Z",
    });
  });
});
