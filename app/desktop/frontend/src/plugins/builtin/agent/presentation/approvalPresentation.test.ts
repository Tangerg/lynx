import { describe, expect, it } from "vitest";
import {
  approvalReversibilityView,
  approvalRiskView,
  approvalScopeViews,
  approvalSettledDecision,
  canSubmitApproval,
} from "./approvalPresentation";

describe("approvalPresentation", () => {
  it("defaults unknown approval risk to medium caution", () => {
    expect(approvalRiskView()).toEqual({
      risk: "medium",
      labelKey: "approval.risk.medium",
      tone: "warning",
    });
  });

  it("maps scopes to presentation tones", () => {
    expect(approvalScopeViews(["read", "write", "delete", "custom"])).toEqual([
      { scope: "read", tone: "neutral" },
      { scope: "write", tone: "warning" },
      { scope: "delete", tone: "danger" },
      { scope: "custom", tone: "neutral" },
    ]);
  });

  it("projects reversibility hints", () => {
    expect(approvalReversibilityView(true)).toEqual({
      labelKey: "approval.reversible",
      tone: "neutral",
    });
    expect(approvalReversibilityView(false)).toEqual({
      labelKey: "approval.permanent",
      tone: "danger",
    });
    expect(approvalReversibilityView(undefined)).toBeNull();
  });

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
