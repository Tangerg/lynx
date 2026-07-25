import { BRAND_ICONS_PANE } from "../../public/panes";
import type { SettingsPaneSpec, WorkspaceViewSpec } from "@/plugins/sdk";

export function iconGalleryWorkspaceView(
  component: WorkspaceViewSpec["component"],
): WorkspaceViewSpec {
  return {
    id: "icon-gallery",
    title: "workspace.view.title.iconGallery",
    icon: "spark",
    order: 60,
    component,
  };
}

export function brandIconsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: BRAND_ICONS_PANE,
    label: "settings.pane.brandIcons",
    group: "advanced",
    icon: "spark",
    order: 110,
    component,
  };
}
