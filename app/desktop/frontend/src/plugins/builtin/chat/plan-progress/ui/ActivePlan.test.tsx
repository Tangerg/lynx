import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { ActivePlan } from "./ActivePlan";

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

describe("ActivePlan disclosure identity", () => {
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
    const { rerender } = render(<ActivePlan />);
    fireEvent.click(screen.getByRole("button", { name: /Expand plan/ }));
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    rerender(<ActivePlan />);
    expect(
      screen.getByRole("button", { name: /Collapse plan/ }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.revision = 2;
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" as const },
      { id: "step-2", text: "Fix", status: "active" as const },
    ];
    rerender(<ActivePlan />);
    expect(screen.getByRole("button", { name: /Expand plan/ }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });

  it("shows a non-dismissible Plan only while its current Run is active", async () => {
    const { rerender } = render(<ActivePlan />);

    expect(screen.queryByRole("button", { name: "Dismiss plan banner" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Expand plan/ })).not.toBeNull();

    model.running = false;
    rerender(<ActivePlan />);

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /Expand plan/ })).toBeNull();
    });
  });
});
