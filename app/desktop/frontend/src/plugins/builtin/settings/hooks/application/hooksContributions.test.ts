import { describe, expect, it } from "vitest";
import { hooksSettingsPane } from "./hooksContributions";

function Component() {
  return null;
}

describe("hooksSettingsPane", () => {
  it("projects the hooks component into the settings pane spec", () => {
    expect(hooksSettingsPane(Component)).toEqual({
      id: "hooks",
      label: "settings.pane.hooks",
      group: "agent",
      icon: "lightning",
      order: 57,
      component: Component,
    });
  });
});
