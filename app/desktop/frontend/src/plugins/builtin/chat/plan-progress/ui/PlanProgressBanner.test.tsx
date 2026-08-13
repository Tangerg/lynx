import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { PlanProgressBanner } from "./PlanProgressBanner";

const model = vi.hoisted(() => ({
  runId: "run-a" as string | null,
  steps: [
    {
      id: "step-1",
      text: "Inspect",
      status: "active" as "active" | "done" | "pending",
    },
  ],
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useCurrentRootRunId: () => model.runId,
}));

vi.mock("@/plugins/builtin/agent/public/plan", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/plan")>()),
  useSessionPlan: () => model.steps,
}));

describe("PlanProgressBanner disclosure identity", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("preserves a choice within one run and resets it for the next run", () => {
    const { rerender } = render(<PlanProgressBanner />);
    fireEvent.click(screen.getByRole("button", { name: /Expand plan/ }));
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" as const },
      { id: "step-2", text: "Fix", status: "active" as const },
    ];
    rerender(<PlanProgressBanner />);
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.runId = "run-b";
    rerender(<PlanProgressBanner />);
    expect(screen.getByRole("button", { name: /Expand plan/ }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });
});
