import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import { DelegatedRunDisclosure } from "./DelegatedRunDisclosure";

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

function run(overrides: Partial<AgentRunView> = {}): AgentRunView {
  return {
    id: "child-run",
    sessionId: "session-1",
    parentRunId: "root-run",
    rootRunId: "root-run",
    spawnedByItemId: "task-item",
    status: "running",
    activeSegmentId: "segment-1",
    outcome: null,
    metrics: {
      steps: 2,
      activeDurationMillis: 10,
      usage: { inputTokens: 3, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: { step: 3, activity: "Reviewing tests" },
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: null,
    ...overrides,
  };
}

function disclosure(value: AgentRunView, onCancel = vi.fn(), onOpenAudit = vi.fn()) {
  return (
    <DelegatedRunDisclosure
      run={value}
      ordinal={1}
      siblingCount={1}
      hasMaterial
      onCancel={onCancel}
      onOpenAudit={onOpenAudit}
    >
      <p>Child narrative</p>
    </DelegatedRunDisclosure>
  );
}

describe("DelegatedRunDisclosure", () => {
  it("starts summarized, exposes an accessible disclosure, and opens on demand", () => {
    render(disclosure(run()));
    const trigger = screen.getByRole("button", { name: /Sub-agent/ });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Child narrative")).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region").getAttribute("aria-labelledby")).toBe(trigger.id);
    expect(screen.getByText("Child narrative")).toBeTruthy();
  });

  it("keeps waiting action visible first and sends exact actions through callbacks", () => {
    const onCancel = vi.fn();
    const onOpenAudit = vi.fn();
    render(
      disclosure(
        run({
          status: "waiting",
          activeSegmentId: null,
          progress: { step: 3, activity: "Waiting for approval" },
        }),
        onCancel,
        onOpenAudit,
      ),
    );

    expect(screen.getByRole("button", { name: /Sub-agent/ }).getAttribute("aria-expanded")).toBe(
      "true",
    );
    expect(screen.getByText("Child narrative")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
    fireEvent.click(screen.getByRole("button", { name: "Open full run audit" }));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onOpenAudit).toHaveBeenCalledOnce();
  });

  it("preserves a user's disclosure choice across authoritative lifecycle refresh", () => {
    const { rerender } = render(disclosure(run()));
    fireEvent.click(screen.getByRole("button", { name: /Sub-agent/ }));

    rerender(
      disclosure(
        run({
          status: "finished",
          activeSegmentId: null,
          progress: null,
          outcome: { type: "completed" },
          finishedAt: "2026-01-01T00:00:01.000Z",
        }),
      ),
    );

    expect(screen.getByRole("button", { name: /Sub-agent/ }).getAttribute("aria-expanded")).toBe(
      "true",
    );
    expect(screen.getByText("Finished")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();
    expect(screen.getByText("Child narrative")).toBeTruthy();
  });
});
