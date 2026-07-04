import { describe, expect, it } from "vitest";
import { schedulesSettingsPane } from "./schedulesContributions";

function Component() {
  return null;
}

describe("schedulesSettingsPane", () => {
  it("projects the schedules component into the settings pane spec", () => {
    expect(schedulesSettingsPane(Component)).toEqual({
      id: "schedules",
      label: "settings.pane.schedules",
      group: "agent",
      icon: "command",
      order: 58,
      component: Component,
    });
  });
});
