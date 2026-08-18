import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { PlanProgressBanner } from "./PlanProgressBanner";

const model = vi.hoisted(() => ({
  sessionId: "ses-a",
  generation: 1,
  revision: 1,
  running: true,
  steps: [
    {
      id: "step-1",
      text: "Inspect",
      status: "active" as "active" | "done" | "pending",
    },
  ],
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useIsCurrentRootRunning: () => model.running,
}));

vi.mock("@/plugins/builtin/agent/public/plan", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/plugins/builtin/agent/public/plan")>();
  return {
    ...original,
    useSessionPlan: () =>
      original.SessionPlan.fromSnapshot(model.sessionId, model.generation, {
        revision: model.revision,
        plan: model.steps,
      }),
  };
});

describe("PlanProgressBanner disclosure identity", () => {
  beforeEach(() => {
    model.sessionId = "ses-a";
    model.generation = 1;
    model.revision = 1;
    model.running = true;
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

  it("shows a non-dismissible Plan only while its current Run is active", () => {
    const { rerender } = render(<PlanProgressBanner />);

    expect(screen.queryByRole("button", { name: "Dismiss plan banner" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Expand plan/ })).not.toBeNull();

    model.running = false;
    rerender(<PlanProgressBanner />);

    expect(screen.queryByRole("button", { name: /Expand plan/ })).toBeNull();
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

  it("does not lend dismissal across projection generations with the same Session and revision", () => {
    const { rerender } = render(<PlanProgressBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss plan banner" }));

    model.generation = 2;
    model.steps = [{ id: "step-1", text: "Successor server plan", status: "active" }];
    rerender(<PlanProgressBanner />);

    expect(screen.queryByText("Successor server plan")).not.toBeNull();
  });
});
