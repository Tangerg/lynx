// useApprovalSubmit now drives the JSON-RPC `runs.approval.submit` method
// (the old HttpPermissionGateway is gone). The one thing worth locking is
// the decision mapping: the UI vocabulary is "approved" | "declined" but
// the wire takes the protocol's "approve" | "deny" (API.md §4.3).

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { Methods } from "@/rpc";
import { useApprovalSubmit } from "./useApprovalSubmit";

afterEach(resetContainer);

function stubSubmit() {
  const submit = vi.fn().mockResolvedValue(undefined);
  setContainer({ methods: () => ({ runs: { approval: { submit } } }) as unknown as Methods });
  return submit;
}

describe("useApprovalSubmit", () => {
  it("maps approved → approve and reflects pending", () => {
    const submit = stubSubmit();
    const { result } = renderHook(() => useApprovalSubmit("req-1"));
    act(() => result.current.submit("approved"));
    expect(submit).toHaveBeenCalledWith({ requestId: "req-1", decision: "approve" });
    expect(result.current.pending).toBe("approved");
  });

  it("maps declined → deny", () => {
    const submit = stubSubmit();
    const { result } = renderHook(() => useApprovalSubmit("req-2"));
    act(() => result.current.submit("declined"));
    expect(submit).toHaveBeenCalledWith({ requestId: "req-2", decision: "deny" });
  });

  it("no-ops without a requestId, and never double-submits", () => {
    const submit = stubSubmit();
    const { result } = renderHook(() => useApprovalSubmit(undefined));
    act(() => result.current.submit("approved"));
    expect(submit).not.toHaveBeenCalled();

    const { result: r2 } = renderHook(() => useApprovalSubmit("req-3"));
    stubSubmit(); // fresh spy on the same container slot
    act(() => r2.current.submit("approved"));
    act(() => r2.current.submit("declined")); // ignored — already pending
    expect(r2.current.pending).toBe("approved");
  });
});
