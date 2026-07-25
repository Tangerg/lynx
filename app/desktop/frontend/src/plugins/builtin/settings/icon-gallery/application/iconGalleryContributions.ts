import type { SettingsPaneSpec, WorkspaceViewSpec } from "@/plugins/sdk";
import type { Translate } from "@/lib/i18n";

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

export function brandIconsSettingsPane(
  t: Translate,
  component: SettingsPaneSpec["component"],
): SettingsPaneSpec {
  return {
    id: "brand-icons",
    label: t("settings.pane.brandIcons"),
    group: "advanced",
    icon: "spark",
    order: 110,
    component,
  };
}
