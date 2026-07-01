// Built-in plugin: an icon gallery for @lobehub/icons. Opens as a
// workspace tab so users can browse the available LLM-model / provider /
// application brand icons. Also registers a curated subset in the
// Settings → "Brand icons" pane.

import { IconGallery } from "./ui/IconGallery";
import { IconShowcase } from "./ui/IconShowcase";
import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { SETTINGS_PANE, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.builtin.icon-gallery",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "icon-gallery",
      title: "workspace.view.title.iconGallery",
      icon: "spark",
      openByDefault: false,
      order: 60,
      component: IconGallery,
    });

    host.extensions.contribute(SETTINGS_PANE, {
      id: "brand-icons",
      label: t("settings.pane.brandIcons"),
      group: "advanced",
      icon: "spark",
      order: 110,
      component: IconShowcase,
    });
  },
});
