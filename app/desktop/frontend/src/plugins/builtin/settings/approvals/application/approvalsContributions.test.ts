import { describe, expect, it } from "vitest";
import { approvalsSettingsPane } from "./approvalsContributions";

function Component() {
  return null;
}

describe("approvalsSettingsPane", () => {
  it("projects the approvals component into the settings pane spec", () => {
    expect(approvalsSettingsPane(Component)).toEqual({
      id: "approvals",
      label: "settings.pane.approvals",
      group: "agent",
      icon: "shield",
      order: 55,
      component: Component,
    });
  });
});
