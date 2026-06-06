// useApprovalSubmit answers a HITL approval interrupt by starting a
// continuation Run via the active session's `resume` action (API.md §6,
// R-model) and optimistically settling the card. The decision maps from the
// UI vocabulary ("approved"|"declined") to the wire pair ("approve"|"deny",
// §6.1 ApprovalResponse).

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { useApprovalSubmit } from "./useApprovalSubmit";

const SID = "ses_1";

function bindResume() {
  const resume = vi.fn();
  useSessionStore.setState({ activeSessionId: SID });
  useAgentStore.getState().setResume(SID, resume);
  return resume;
}

afterEach(() => {
  useAgentStore.getState().dropSession(SID);
});
beforeEach(() => {
  useAgentStore.getState().dropSession(SID);
});

describe("useApprovalSubmit", () => {
  it("maps approved → approve and latches pending", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_1"));
    act(() => result.current.submit("approved"));
    expect(resume).toHaveBeenCalledWith("run_1", [
      { itemId: "item_1", response: { type: "approval", decision: "approve" } },
    ]);
    expect(result.current.pending).toBe("approved");
  });

  it("maps declined → deny", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_2"));
    act(() => result.current.submit("declined"));
    expect(resume).toHaveBeenCalledWith("run_1", [
      { itemId: "item_2", response: { type: "approval", decision: "deny" } },
    ]);
  });

  it("forwards editedArgs only when provided (approve-with-modified-args)", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_e"));
    act(() => result.current.submit("approved", { path: "/safe" }));
    expect(resume).toHaveBeenCalledWith("run_1", [
      {
        itemId: "item_e",
        response: { type: "approval", decision: "approve", editedArgs: { path: "/safe" } },
      },
    ]);
  });

  it("no-ops without a parentRunId/itemId, and never double-submits", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit(undefined, undefined));
    act(() => result.current.submit("approved"));
    expect(resume).not.toHaveBeenCalled();

    const { result: r2 } = renderHook(() => useApprovalSubmit("run_1", "item_3"));
    act(() => r2.current.submit("approved"));
    act(() => r2.current.submit("declined")); // ignored — already pending
    expect(r2.current.pending).toBe("approved");
    expect(resume).toHaveBeenCalledTimes(1);
  });
});
