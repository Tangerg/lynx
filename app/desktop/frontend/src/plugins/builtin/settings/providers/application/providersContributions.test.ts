import { describe, expect, it } from "vitest";
import { providersSettingsPane } from "./providersContributions";

function Component() {
  return null;
}

describe("providersSettingsPane", () => {
  it("projects the providers component into the settings pane spec", () => {
    expect(providersSettingsPane(Component)).toEqual({
      id: "providers",
      label: "settings.pane.providers",
      group: "models",
      icon: "spark",
      order: 50,
      component: Component,
    });
  });
});
