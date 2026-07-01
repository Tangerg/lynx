import { beforeEach, describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";

const toolCall = (over: Partial<ToolCall> & Pick<ToolCall, "id" | "name">): ToolCall => ({
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("openWorkspaceViewForTool", () => {
  beforeEach(() => {
    useWorkspaceNavigationStore.setState({
      splitViewId: null,
      activeMainView: null,
      mainViewTabs: [],
      selectedToolId: "",
      activeFile: "",
    });
  });

  it("reports whether a tool has a workspace view", () => {
    expect(hasWorkspaceViewForTool(toolCall({ id: "t1", name: "shell" }))).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t2", name: "read" }))).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t3", name: "grep" }))).toBe(false);
  });

  it("opens a command tool beside chat as the terminal split, leaving activeMainView null", () => {
    openWorkspaceViewForTool(toolCall({ id: "t1", name: "shell", fn: "ls -la" }));
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBe("terminal");
    expect(s.activeMainView).toBeNull();
    expect(s.selectedToolId).toBe("t1");
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    openWorkspaceViewForTool(toolCall({ id: "t2", name: "edit", fn: "src/app.ts" }));
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBe("diff");
    expect(s.activeMainView).toBeNull();
    expect(s.activeFile).toBe("src/app.ts");
  });

  it("does not feed a multi-file edit label to the diff's active-file focus", () => {
    openWorkspaceViewForTool(toolCall({ id: "t3", name: "edit", fn: "3 files" }));
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBe("diff");
    expect(s.activeFile).toBe("");
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(toolCall({ id: "t4", name: "grep", fn: "foo" }));
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBeNull();
    expect(s.selectedToolId).toBe("");
  });
});
