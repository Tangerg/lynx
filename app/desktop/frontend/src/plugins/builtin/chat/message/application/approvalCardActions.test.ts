import { describe, expect, it } from "vitest";
import { approvalSubmitOptions, canRegisterApprovalActions } from "./approvalCardActions";

describe("approvalSubmitOptions", () => {
  it("omits the options object when approval has no extra payload", () => {
    expect(approvalSubmitOptions({})).toBeUndefined();
  });

  it("preserves edited args and remember scope", () => {
    expect(approvalSubmitOptions({ editedArgs: {}, rememberScope: "project" })).toEqual({
      editedArgs: {},
      rememberScope: "project",
    });
  });
});

describe("canRegisterApprovalActions", () => {
  it("registers shortcuts only for an open resumable approval", () => {
    expect(
      canRegisterApprovalActions({
        parentRunId: "run",
        itemId: "item",
        status: "requires-action",
      }),
    ).toBe(true);
    expect(
      canRegisterApprovalActions({
        parentRunId: "run",
        itemId: "item",
        status: "complete",
      }),
    ).toBe(false);
    expect(
      canRegisterApprovalActions({
        parentRunId: undefined,
        itemId: "item",
        status: "requires-action",
      }),
    ).toBe(false);
  });
});
