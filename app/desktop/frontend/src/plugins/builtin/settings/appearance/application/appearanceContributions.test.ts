import { describe, expect, it } from "vitest";
import { appearanceSettingsPane } from "./appearanceContributions";

function Component() {
  return null;
}

describe("appearanceSettingsPane", () => {
  it("projects the appearance component into the settings pane spec", () => {
    expect(appearanceSettingsPane(Component)).toEqual({
      id: "appearance",
      label: "settings.pane.appearance",
      group: "general",
      icon: "spark",
      order: 0,
      component: Component,
    });
  });
});
