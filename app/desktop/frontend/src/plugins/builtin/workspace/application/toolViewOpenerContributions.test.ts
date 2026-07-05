import { describe, expect, it } from "vitest";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";
import { workspaceToolViewOpener } from "./toolViewOpenerContributions";

describe("workspaceToolViewOpener", () => {
  it("projects workspace tool routing into the tool view opener spec", () => {
    expect(workspaceToolViewOpener()).toEqual({
      id: "workspace-tool-view",
      order: 0,
      predicate: hasWorkspaceViewForTool,
      open: openWorkspaceViewForTool,
    });
  });
});
