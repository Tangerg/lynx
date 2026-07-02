import { beforeEach, describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";
import { workspaceCommandActivitiesFromAgentTools } from "./toolActivity";

const toolCall = (over: Partial<ToolCall> & Pick<ToolCall, "id" | "name">): ToolCall => ({
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("openWorkspaceViewForTool", () => {
  beforeEach(() => {
    useWorkspaceSurfaceStore.setState({
      activeMainView: null,
      mainViewTabs: [],
    });
    useContextDockStore.setState({
      splitViewId: null,
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
    expect(useContextDockStore.getState().splitViewId).toBe("terminal");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
    expect(useContextDockStore.getState().selectedToolId).toBe("t1");
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    openWorkspaceViewForTool(toolCall({ id: "t2", name: "edit", fn: "src/app.ts" }));
    expect(useContextDockStore.getState().splitViewId).toBe("diff");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
    expect(useContextDockStore.getState().activeFile).toBe("src/app.ts");
  });

  it("does not feed a multi-file edit label to the diff's active-file focus", () => {
    openWorkspaceViewForTool(toolCall({ id: "t3", name: "edit", fn: "3 files" }));
    expect(useContextDockStore.getState().splitViewId).toBe("diff");
    expect(useContextDockStore.getState().activeFile).toBe("");
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(toolCall({ id: "t4", name: "grep", fn: "foo" }));
    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
    expect(useContextDockStore.getState().selectedToolId).toBe("");
  });

  it("projects command tools into a workspace command view model", () => {
    expect(
      workspaceCommandActivitiesFromAgentTools({
        t1: toolCall({
          id: "t1",
          name: "shell",
          fn: "npm test",
          status: "err",
          result: "failed",
          outputTruncated: true,
          exitCode: 1,
        }),
        t2: toolCall({ id: "t2", name: "read", fn: "src/app.ts" }),
      }),
    ).toEqual([
      {
        id: "t1",
        command: "npm test",
        status: "failed",
        output: "failed",
        outputTruncated: true,
        exitCode: 1,
      },
    ]);
  });
});
