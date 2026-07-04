import { describe, expect, it } from "vitest";
import { brandIconsSettingsPane, iconGalleryWorkspaceView } from "./iconGalleryContributions";

function Component() {
  return null;
}

describe("iconGalleryWorkspaceView", () => {
  it("projects the gallery component into the workspace view spec", () => {
    expect(iconGalleryWorkspaceView(Component)).toEqual({
      id: "icon-gallery",
      title: "workspace.view.title.iconGallery",
      icon: "spark",
      order: 60,
      component: Component,
    });
  });
});

describe("brandIconsSettingsPane", () => {
  it("projects the showcase component into the settings pane spec", () => {
    expect(brandIconsSettingsPane((key) => `t:${key}`, Component)).toEqual({
      id: "brand-icons",
      label: "t:settings.pane.brandIcons",
      group: "advanced",
      icon: "spark",
      order: 110,
      component: Component,
    });
  });
});
