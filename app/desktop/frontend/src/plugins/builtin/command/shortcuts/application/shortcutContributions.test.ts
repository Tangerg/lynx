import { describe, expect, it } from "vitest";
import { shortcutsProviderSlot, shortcutsSettingsPane } from "./shortcutContributions";

function Component() {
  return null;
}

describe("shortcutsProviderSlot", () => {
  it("projects the provider component into the overlay slot spec", () => {
    expect(shortcutsProviderSlot(Component)).toEqual({
      id: "shortcuts-provider",
      order: 50,
      component: Component,
    });
  });
});

describe("shortcutsSettingsPane", () => {
  it("projects the pane component into the settings pane spec", () => {
    expect(shortcutsSettingsPane(Component)).toEqual({
      id: "shortcuts",
      label: "Shortcuts",
      icon: "command",
      order: 50,
      component: Component,
    });
  });
});
