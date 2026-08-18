import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalCard } from "./ApprovalCard";

vi.mock("../../application/approvalArgsEditor", () => ({
  useApprovalArgsEditor: () => ({
    editing: false,
    argsText: "",
    invalid: false,
    setEditing: vi.fn(),
    setArgsText: vi.fn(),
  }),
}));

vi.mock("../../application/approvalCardActions", () => ({
  useApprovalCardActions: () => ({
    pending: undefined,
    disabled: false,
    approve: vi.fn(),
    decline: vi.fn(),
  }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("ApprovalCard actions", () => {
  it("orders the deny action before the primary approval action", () => {
    render(
      <ApprovalCard
        status="requires-action"
        runId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="npm test"
        reason="Run the test suite."
      />,
    );

    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual([
      "Deny",
      "Allow once",
    ]);
  });
});
