import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { PlanProgressBanner } from "./PlanProgressBanner";

const model = vi.hoisted(() => ({
  sessionId: "ses-a",
  revision: 1,
  steps: [
    {
      id: "step-1",
      text: "Inspect",
      status: "active" as "active" | "done" | "pending",
    },
  ],
}));

vi.mock("@/plugins/builtin/agent/public/plan", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/plugins/builtin/agent/public/plan")>();
  return {
    ...original,
    useSessionPlan: () =>
      original.SessionPlan.fromSnapshot(model.sessionId, {
        revision: model.revision,
        plan: model.steps,
      }),
  };
});

describe("PlanProgressBanner disclosure identity", () => {
  beforeEach(() => {
    model.sessionId = "ses-a";
    model.revision = 1;
    model.steps = [{ id: "step-1", text: "Inspect", status: "active" }];
  });

  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("preserves a choice within one replacement and resets it for its successor", () => {
    const { rerender } = render(<PlanProgressBanner />);
    fireEvent.click(screen.getByRole("button", { name: /Expand plan/ }));
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    rerender(<PlanProgressBanner />);
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.revision = 2;
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" as const },
      { id: "step-2", text: "Fix", status: "active" as const },
    ];
    rerender(<PlanProgressBanner />);
    expect(screen.getByRole("button", { name: /Expand plan/ }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });

  it("does not lend a dismissed Plan's material state to a replacement in the same Run", () => {
    const { rerender } = render(<PlanProgressBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss plan banner" }));

    model.revision = 2;
    model.steps = [{ id: "step-1", text: "Replacement plan", status: "active" }];
    rerender(<PlanProgressBanner />);

    expect(screen.queryByText("Replacement plan")).not.toBeNull();
  });

  it("does not lend dismissal across Sessions with the same Plan revision", () => {
    const { rerender } = render(<PlanProgressBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss plan banner" }));

    model.sessionId = "ses-b";
    model.steps = [{ id: "step-1", text: "Other Session plan", status: "active" }];
    rerender(<PlanProgressBanner />);

    expect(screen.queryByText("Other Session plan")).not.toBeNull();
  });
});
