import { describe, expect, it } from "vitest";
import type { WorkspaceToolActivity } from "./toolActivity";
import { decideWorkspaceToolRoute, hasWorkspaceToolView } from "./toolRouteDecision";

const tool = (over: Partial<WorkspaceToolActivity>): WorkspaceToolActivity => ({
  id: "t1",
  category: "inline",
  label: "",
  ...over,
});

describe("decideWorkspaceToolRoute", () => {
  it("routes command tools to the terminal view", () => {
    expect(decideWorkspaceToolRoute(tool({ category: "command", label: "ls -la" }))).toEqual({
      view: { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" },
    });
  });

  it("routes edit tools to the diff view and exposes the focused file", () => {
    expect(decideWorkspaceToolRoute(tool({ category: "fileEdit", label: "src/app.ts" }))).toEqual({
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: "src/app.ts",
    });
  });

  it("does not treat multi-file labels as file paths", () => {
    expect(decideWorkspaceToolRoute(tool({ category: "fileEdit", label: "3 files" }))).toEqual({
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: undefined,
    });
  });

  it("does not route inline-only tool categories", () => {
    const search = tool({ category: "inline", label: "needle" });

    expect(hasWorkspaceToolView(search)).toBe(false);
    expect(decideWorkspaceToolRoute(search)).toBeNull();
  });
});
