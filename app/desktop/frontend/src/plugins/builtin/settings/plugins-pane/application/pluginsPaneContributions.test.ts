import { describe, expect, it } from "vitest";
import { pluginsSettingsPane } from "./pluginsPaneContributions";

function Component() {
  return null;
}

describe("pluginsSettingsPane", () => {
  it("projects the plugins component into the settings pane spec", () => {
    expect(pluginsSettingsPane(Component)).toEqual({
      id: "plugins",
      label: "settings.pane.plugins",
      group: "integrations",
      icon: "tool",
      order: 99,
      component: Component,
    });
  });
});
