import type { ReactNode, Ref } from "react";
import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";

const projection = vi.hoisted(() => ({
  toolCalls: {} as Record<string, ToolCall>,
  selectedToolId: "cmd-1",
  selectTool: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useActiveSessionToolCalls: () => projection.toolCalls,
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  useSelectedWorkspaceToolId: () => projection.selectedToolId,
  useSelectWorkspaceTool: () => projection.selectTool,
}));

vi.mock("./views/WorkspaceViewLayout", () => ({
  WorkspaceViewLayout: ({
    children,
    scrollRef,
  }: {
    children: ReactNode;
    scrollRef: Ref<HTMLDivElement>;
  }) => (
    <div ref={scrollRef} data-testid="terminal-scroll">
      {children}
    </div>
  ),
}));

import { TerminalWorkspaceSurface } from "./terminal";

function command(result: string, status: ToolCall["status"]): ToolCall {
  return {
    id: "cmd-1",
    runId: "run-1",
    name: "shell",
    fn: "shell",
    args: '{"command":"test"}',
    command: "test",
    result,
    status,
  };
}

function fileEdit(): ToolCall {
  return {
    id: "file-edit-1",
    runId: "run-1",
    name: "apply_patch",
    fn: "src/app.ts",
    args: '{"path":"src/app.ts"}',
    status: "ok",
  };
}

beforeEach(() => {
  projection.toolCalls = { "cmd-1": command("1234567", "running") };
  projection.selectedToolId = "cmd-1";
  projection.selectTool.mockClear();
});

describe("TerminalWorkspaceSurface", () => {
  it("does not replace an exact non-command Tool selection while mounted", () => {
    projection.toolCalls = {
      "cmd-1": command("1234567", "running"),
      "file-edit-1": fileEdit(),
    };
    projection.selectedToolId = "file-edit-1";

    render(<TerminalWorkspaceSurface />);

    expect(projection.selectTool).not.toHaveBeenCalled();
    expect(screen.getByText("test").closest("[data-command-selected]")).toBeTruthy();
  });

  it("tails an equal-length authoritative replacement while pinned", () => {
    const view = render(<TerminalWorkspaceSurface />);
    const scroller = screen.getByTestId("terminal-scroll");
    let scrollHeight = 80;
    Object.defineProperty(scroller, "scrollHeight", {
      configurable: true,
      get: () => scrollHeight,
    });
    scroller.scrollTop = 10;

    act(() => {
      scrollHeight = 160;
      projection.toolCalls = { "cmd-1": command("1\n2\n3\n4", "ok") };
      view.rerender(<TerminalWorkspaceSurface />);
    });

    expect(scroller.scrollTop).toBe(160);
  });
});
