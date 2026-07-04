import { describe, expect, it } from "vitest";
import { personalizationSettingsPane } from "./personalizationContributions";

function Component() {
  return null;
}

describe("personalizationSettingsPane", () => {
  it("projects the personalization component into the settings pane spec", () => {
    expect(personalizationSettingsPane(Component)).toEqual({
      id: "personalization",
      label: "settings.pane.personalization",
      group: "general",
      icon: "user",
      order: 1,
      component: Component,
    });
  });
});
