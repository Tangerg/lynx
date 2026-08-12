import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { useToggleFavorite } from "./favoriteSession";
import { useRenameSession } from "./renameSession";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";

let restoreRuntime: (() => void) | undefined;

function session(): AgentSessionSummary {
  return {
    id: "ses_deleted",
    revision: 3,
    title: "before",
    status: "idle",
    model: "gpt-5",
    cwd: "",
    time: "2026-08-12T00:00:00Z",
  };
}

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  vi.restoreAllMocks();
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("optimistic Session summary mutations", () => {
  it.each([
    {
      name: "rename",
      run: async () => {
        const { result } = renderHook(() => useRenameSession());
        await result.current("ses_deleted", 3, "after");
      },
    },
    {
      name: "favorite",
      run: async () => {
        const { result } = renderHook(() => useToggleFavorite());
        await result.current("ses_deleted", 3, true);
      },
    },
  ])("revalidates authoritative membership when $name loses a delete race", async ({ run }) => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession: vi.fn().mockRejectedValue(new Error("deleted concurrently")),
    } as unknown as AgentRuntimeGateway);

    await run();

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });
});
