import { describe, expect, it } from "vitest";
import { usageSettingsPane } from "./usageContributions";

function Component() {
  return null;
}

describe("usageSettingsPane", () => {
  it("projects the usage component into the settings pane spec", () => {
    expect(usageSettingsPane(Component)).toEqual({
      id: "usage",
      label: "settings.pane.usage",
      group: "models",
      icon: "chart",
      order: 55,
      component: Component,
    });
  });
});
