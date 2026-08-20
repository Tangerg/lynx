import { beforeEach, describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useContextDockStore, WorkspaceFileFocus } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";
import { workspaceCommandActivitiesFromAgentTools } from "./toolActivity";

const toolCall = ({
  runId = "run_1",
  ...over
}: Partial<ToolCall> & Pick<ToolCall, "id" | "name">): ToolCall => ({
  runId,
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("openWorkspaceViewForTool", () => {
  beforeEach(() => {
    navigator().go({ view: null });
    useContextDockStore.setState({
      dockViewIds: [],
      lastViewId: null,
      selectedToolId: "",
      fileFocus: WorkspaceFileFocus.empty(),
    });
  });

  it("reports whether a tool has a workspace view", () => {
    expect(hasWorkspaceViewForTool(toolCall({ id: "t1", name: "shell" }))).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t2", name: "read" }))).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t3", name: "grep" }))).toBe(false);
  });

  it("opens a command tool beside chat as the terminal split, leaving activeMainView null", () => {
    openWorkspaceViewForTool(toolCall({ id: "t1", name: "shell", fn: "ls -la" }));
    expect(navigator().get().dock).toBe("terminal");
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().selectedToolId).toBe("t1");
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    openWorkspaceViewForTool(
      toolCall({ id: "t2", name: "apply_patch", fn: "src/app.ts", fnKind: "path" }),
    );
    expect(navigator().get().dock).toBe("diff");
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().fileFocus).toMatchObject({
      path: "src/app.ts",
      revision: 1,
    });
  });

  it("does not feed a multi-file patch label to the diff's active-file focus", () => {
    useContextDockStore.getState().focusFile("src/old.ts");
    openWorkspaceViewForTool(toolCall({ id: "t3", name: "apply_patch", fn: "apply_patch" }));
    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState().fileFocus).toMatchObject({ path: "", revision: 2 });
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(toolCall({ id: "t4", name: "grep", fn: "foo" }));
    expect(navigator().get().dock).toBeNull();
    expect(navigator().get().view).toBeNull();
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
        exitCode: 1,
      },
    ]);
  });
});
