import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { AGENT_SESSIONS_KEY } from "./sessionQueries";
import { useRelocateSession } from "./relocateSession";

let restoreRuntime: (() => void) | undefined;

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  vi.restoreAllMocks();
});

describe("useRelocateSession", () => {
  it("revalidates Session truth after an ambiguous conditional-write failure", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession: vi.fn().mockRejectedValue(new Error("response lost after commit")),
    } as unknown as AgentRuntimeGateway);
    const { result } = renderHook(() => useRelocateSession());

    await expect(result.current("ses_move", 4, "/next")).resolves.toBe(false);

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });
});
