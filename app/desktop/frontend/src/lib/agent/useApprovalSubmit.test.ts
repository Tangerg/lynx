// useApprovalSubmit answers a HITL approval interrupt by starting a
// continuation Run via the owning session's `resume` action (API.md §6,
// R-model). The card's optimistic settle is local `pending`; the store settle
// (resolveInterrupt) commits only once the run starts, and rolls back on a
// channel-a failure. The decision maps from the UI vocabulary
// ("approved"|"declined") to the wire pair ("approve"|"deny", §6.1).

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { useApprovalSubmit } from "./useApprovalSubmit";

const SID = "ses_1";

// resetSession seeds the slice before setResume — mirrors useAgentSession,
// which resets at mount then binds the imperative actions. Required now that
// the store refuses to resurrect a dropped/absent session (see agentStore).
function bindResume(impl?: (...args: unknown[]) => void) {
  const resume = impl ? vi.fn(impl) : vi.fn();
  useSessionStore.setState({ activeSessionId: SID });
  useAgentStore.getState().resetSession(SID);
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
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_1", response: { type: "approval", decision: "approve" } }],
      expect.any(Function),
      expect.any(Function),
    );
    expect(result.current.pending).toBe("approved");
  });

  it("maps declined → deny", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_2"));
    act(() => result.current.submit("declined"));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_2", response: { type: "approval", decision: "deny" } }],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("forwards editedArgs only when provided (approve-with-modified-args)", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_e"));
    act(() => result.current.submit("approved", { path: "/safe" }));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [
        {
          itemId: "item_e",
          response: { type: "approval", decision: "approve", editedArgs: { path: "/safe" } },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
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

  it("commits resolveInterrupt only after the run starts (onSettled)", () => {
    // resume invokes the success callback synchronously (run accepted).
    bindResume((_run, _resp, onSettled) => (onSettled as () => void)());
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_ok"));
    act(() => result.current.submit("approved"));
    expect(spy).toHaveBeenCalledWith(SID, "item_ok", { decision: "approved" });
    spy.mockRestore();
  });

  it("rolls back pending and does NOT resolve when the resume rejects (channel-a)", () => {
    // resume invokes the failure callback synchronously (runs.resume rejected).
    bindResume((_run, _resp, _onSettled, onStartError) => (onStartError as () => void)());
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_fail"));
    act(() => result.current.submit("approved"));
    expect(spy).not.toHaveBeenCalled();
    expect(result.current.pending).toBeNull(); // card back to actionable — retryable
    spy.mockRestore();
  });
});
