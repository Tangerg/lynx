import { describe, expect, it } from "vitest";
import { diagnosticsWorkspaceView } from "./diagnosticsContributions";

function Component() {
  return null;
}

describe("diagnosticsWorkspaceView", () => {
  it("projects the diagnostics component into a stable workspace view spec", () => {
    expect(diagnosticsWorkspaceView(Component)).toEqual({
      id: "diagnostics",
      title: "workspace.view.title.diagnostics",
      icon: "spark",
      order: 90,
      component: Component,
    });
  });
});
