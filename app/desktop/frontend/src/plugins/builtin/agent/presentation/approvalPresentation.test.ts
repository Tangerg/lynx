import { describe, expect, it } from "vitest";
import { approvalSettledDecision, canSubmitApproval } from "../public/messagePresentation";

describe("approvalPresentation", () => {
  it("prefers completed decisions over pending decisions", () => {
    expect(approvalSettledDecision("complete", "approved", "declined")).toBe("approved");
    expect(approvalSettledDecision("requires-action", undefined, "declined")).toBe("declined");
    expect(approvalSettledDecision("requires-action", undefined, null)).toBeNull();
  });

  it("allows submit only for open resumable approval interrupts", () => {
    expect(
      canSubmitApproval({
        runId: "run",
        itemId: "item",
        pending: null,
        status: "requires-action",
      }),
    ).toBe(true);
    expect(
      canSubmitApproval({
        runId: "run",
        itemId: "item",
        pending: "approved",
        status: "requires-action",
      }),
    ).toBe(false);
    expect(
      canSubmitApproval({ runId: "run", itemId: "item", pending: null, status: "complete" }),
    ).toBe(false);
  });
});
