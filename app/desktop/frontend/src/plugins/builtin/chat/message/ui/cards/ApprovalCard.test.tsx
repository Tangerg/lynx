import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApprovalCard } from "./ApprovalCard";

const actions = vi.hoisted(() => ({
  approve: vi.fn(),
  decline: vi.fn(),
}));

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
    approve: actions.approve,
    decline: actions.decline,
  }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("ApprovalCard actions", () => {
  afterEach(() => vi.clearAllMocks());

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

  it("uses the Codex request hierarchy without local danger chrome", () => {
    const { container } = render(
      <ApprovalCard
        status="requires-action"
        runId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="rm -rf node_modules && pnpm install"
        reason="Reinstall dependencies from the lockfile."
        rememberable
      />,
    );

    expect(container.querySelector('[data-slot="approval-surface"]')).toBeTruthy();
    expect(screen.getByText("Terminal", { exact: true })).toBeTruthy();
    expect(
      screen.getByText("Reinstall dependencies from the lockfile.", { exact: true }),
    ).toBeTruthy();
    expect(screen.queryByText("Approval required", { exact: true })).toBeNull();
    expect(screen.queryByText(/Potentially destructive/)).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("keeps remembered approval scopes behind the primary split action", () => {
    render(
      <ApprovalCard
        status="requires-action"
        runId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="npm test"
        reason="Run the test suite."
        rememberable
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Approval options" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Allow for this session" }));
    expect(actions.approve).toHaveBeenCalledWith("session");
  });
});
