import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRunView, Message, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { TurnFacts } from "@/plugins/builtin/agent/public/conversation";
import { MessageContext } from "@/plugins/sdk/messageContext";
import type { BlockCtx } from "./BlockRenderer";
import { renderBlock } from "./BlockRenderer";

// The transcript's shared context holds no session data by design, so every test
// needs the same inert one — the interesting half is `facts`.
const CTX: BlockCtx = {
  onSelectTool: vi.fn(),
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

const agentRunCommands = vi.hoisted(() => ({ cancel: vi.fn() }));
vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: agentRunCommands.cancel,
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

function run(
  id: string,
  parentRunId: string,
  rootRunId: string,
  spawnedByItemId: string,
  status: AgentRunView["status"] = "finished",
): AgentRunView {
  return {
    id,
    sessionId: "session-1",
    parentRunId,
    rootRunId,
    spawnedByItemId,
    status,
    activeSegmentId: status === "running" ? `segment-${id}` : null,
    outcome: status === "finished" ? { type: "completed" } : null,
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-01-01T00:00:01.000Z" : null,
  };
}

function tool(id: string): ToolCall {
  return {
    id,
    runId: "root-run",
    name: "task",
    fn: "Delegate work",
    args: "{}",
    status: "ok",
  };
}

function message(id: string, runId: string, toolCallId: string): Message {
  return {
    id,
    runId,
    role: "assistant",
    blocks: [{ kind: "tool", toolCallId }],
  };
}

function renderRootTool(toolCallId: string, facts: TurnFacts) {
  const rootMessage = message("root-message", "root-run", toolCallId);
  return render(
    <MessageContext.Provider value={{ sessionId: "session-1", message: rootMessage }}>
      {renderBlock(rootMessage.blocks[0]!, 0, facts, CTX)}
    </MessageContext.Provider>,
  );
}

describe("delegated Run rendering", () => {
  it("mounts child and nested narratives under their exact parent task Items", () => {
    const parentTool = tool("task-root");
    const nestedTool = { ...tool("task-child"), runId: "child-run" };
    // One turn's facts reach through delegation: the nested task's call and the run IT
    // spawned are in here because the projection walks the whole delegation chain when
    // it slices a row.
    const facts: TurnFacts = {
      toolCalls: {
        [parentTool.id]: parentTool,
        [nestedTool.id]: nestedTool,
      },
      delegatedRuns: {
        [parentTool.id]: [
          {
            run: run("child-run", "root-run", "root-run", parentTool.id),
            messages: [message("child-message", "child-run", nestedTool.id)],
          },
        ],
        [nestedTool.id]: [
          {
            run: run("nested-run", "child-run", "root-run", nestedTool.id),
            messages: [],
          },
        ],
      },
    };

    renderRootTool(parentTool.id, facts);
    expect(screen.getAllByText("Sub-agent")).toHaveLength(1);
    expect(screen.queryByText("No narrative material yet.")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Sub-agent/ }));
    expect(screen.getAllByText("Sub-agent")).toHaveLength(2);

    const taskAnchors = document.querySelectorAll("#task-root, #task-child");
    expect(taskAnchors).toHaveLength(2);
  });

  it("targets the exact descendant Run when its disclosure is canceled", () => {
    agentRunCommands.cancel.mockClear();
    const parentTool = tool("task-root");
    const facts: TurnFacts = {
      toolCalls: { [parentTool.id]: parentTool },
      delegatedRuns: {
        [parentTool.id]: [
          {
            run: run("child-run", "root-run", "root-run", parentTool.id, "running"),
            messages: [],
          },
        ],
      },
    };

    renderRootTool(parentTool.id, facts);
    fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));

    expect(agentRunCommands.cancel).toHaveBeenCalledOnce();
    expect(agentRunCommands.cancel).toHaveBeenCalledWith({
      sessionId: "session-1",
      runId: "child-run",
    });
  });
});
