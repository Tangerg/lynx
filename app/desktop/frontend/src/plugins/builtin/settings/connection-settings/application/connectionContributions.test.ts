import { describe, expect, it } from "vitest";
import { connectionSettingsPane } from "./connectionContributions";

function Component() {
  return null;
}

describe("connectionSettingsPane", () => {
  it("projects the connection component into the settings pane spec", () => {
    expect(connectionSettingsPane(Component)).toEqual({
      id: "connection",
      label: "settings.pane.connection",
      group: "general",
      icon: "globe",
      order: 5,
      component: Component,
    });
  });
});
