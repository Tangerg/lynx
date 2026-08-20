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
      view: "terminal",
    });
  });

  it("routes patch tools to the diff view and exposes an authoritative path label", () => {
    expect(
      decideWorkspaceToolRoute(
        tool({ category: "fileEdit", label: "src/app.ts", labelKind: "path" }),
      ),
    ).toEqual({
      view: "diff",
      fileFocus: "src/app.ts",
    });
  });

  it("does not treat a multi-file patch's text label as a file path", () => {
    expect(decideWorkspaceToolRoute(tool({ category: "fileEdit", label: "apply_patch" }))).toEqual({
      view: "diff",
      fileFocus: "",
    });
  });

  it("does not route inline-only tool categories", () => {
    const search = tool({ category: "inline", label: "needle" });

    expect(hasWorkspaceToolView(search)).toBe(false);
    expect(decideWorkspaceToolRoute(search)).toBeNull();
  });
});
