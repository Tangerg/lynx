import { beforeEach, describe, expect, it } from "vitest";
import { INITIAL_VIEW_STATE, type ToolCall } from "@/protocol/run/viewState";
import { useAgentStore } from "./agentStore";
import { useSessionStore } from "./sessionStore";
import { openViewForTool } from "./toolRouting";

const SID = "s1";

// Seed the active session + one tool call into the agent store so
// getCurrentSessionView() (which openViewForTool reads) resolves it.
function seedTool(tool: ToolCall) {
  useSessionStore.setState({ activeSessionId: SID });
  useAgentStore.setState({
    sessions: {
      [SID]: {
        view: { ...INITIAL_VIEW_STATE, toolCalls: { [tool.id]: tool } },
        viewEpoch: 0,
        stop: null,
        send: null,
        resume: null,
      },
    },
  });
}

const toolCall = (over: Partial<ToolCall> & Pick<ToolCall, "id" | "name">): ToolCall => ({
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("openViewForTool routes a clicked tool BESIDE chat (split), never as a full tab", () => {
  beforeEach(() => {
    useSessionStore.setState({
      splitViewId: null,
      activeMainView: null,
      mainViewTabs: [],
      selectedToolId: "",
      activeFile: "",
    });
  });

  it("opens a command tool beside chat as the terminal split, leaving activeMainView null", () => {
    seedTool(toolCall({ id: "t1", name: "bash", fn: "ls -la" }));
    openViewForTool("t1");
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBe("terminal");
    expect(s.activeMainView).toBeNull(); // chat keeps the other half — NOT a full tab
    expect(s.selectedToolId).toBe("t1");
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    seedTool(toolCall({ id: "t2", name: "edit", fn: "src/app.ts" }));
    openViewForTool("t2");
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBe("diff");
    expect(s.activeMainView).toBeNull();
    expect(s.activeFile).toBe("src/app.ts");
  });

  it("does not feed a multi-file edit label to the diff's active-file focus", () => {
    seedTool(toolCall({ id: "t3", name: "edit", fn: "3 files" }));
    openViewForTool("t3");
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBe("diff");
    expect(s.activeFile).toBe(""); // "3 files" is a label, not a path
  });

  it("promotes NO view for search/lsp/etc — used to wrongly fall through to Diff", () => {
    seedTool(toolCall({ id: "t4", name: "grep", fn: "foo" }));
    openViewForTool("t4");
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBeNull();
    expect(s.selectedToolId).toBe("t4"); // selection is still recorded
  });

  it("is a no-op when the tool id is not in the current session view", () => {
    seedTool(toolCall({ id: "t1", name: "bash" }));
    openViewForTool("missing");
    expect(useSessionStore.getState().splitViewId).toBeNull();
  });
});
