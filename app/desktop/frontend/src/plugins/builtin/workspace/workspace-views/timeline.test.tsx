import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const projection = vi.hoisted(() => ({
  runtimeAvailable: false,
  cancelRun: vi.fn(),
}));

const running: AgentRunView = {
  id: "run-1",
  sessionId: "session-1",
  parentRunId: null,
  rootRunId: "run-1",
  spawnedByItemId: null,
  status: "running",
  activeSegmentId: "segment-1",
  outcome: null,
  metrics: {
    steps: 2,
    activeDurationMillis: 10,
    usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
  },
  progress: { step: 3, activity: "Inspecting" },
  createdAt: "2026-01-01T00:00:00.000Z",
  finishedAt: null,
};

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: projection.cancelRun,
  useActiveSessionRunTree: () => [{ run: running, children: [] }],
  useActiveSessionTimeline: () => [],
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => projection.runtimeAvailable,
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  locateWorkspaceTool: vi.fn(),
  selectWorkspaceChat: vi.fn(),
}));

vi.mock("./views/WorkspaceViewLayout", () => ({
  WorkspaceViewLayout: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

import { timelineView } from "./timeline";

describe("Timeline runtime actions", () => {
  it("does not offer an active cancel command while the Runtime is unavailable", async () => {
    await loadPluginsForTest(timelineView);
    const View = lookupExtensionPoint(WORKSPACE_VIEW).find(
      (view) => view.id === "timeline",
    )!.component;

    render(<View />);

    const cancel = screen.getByRole("button", { name: "Cancel this run" });
    expect((cancel as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(cancel);
    expect(projection.cancelRun).not.toHaveBeenCalled();
  });
});
