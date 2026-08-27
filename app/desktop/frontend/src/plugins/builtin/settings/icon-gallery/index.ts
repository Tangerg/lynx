// Built-in plugin: an icon gallery for @lobehub/icons. Opens as a
// workspace tab so users can browse the available LLM-model / provider /
// application brand icons. Also registers a curated subset in the
// Settings → "Brand icons" pane.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { registerSettingsPane } from "../public";
import { BRAND_ICONS_PANE } from "../public/panes";

const IconGallery = lazy(() =>
  import("./ui/IconGallery").then(({ IconGallery }) => ({ default: IconGallery })),
);
const IconShowcase = lazy(() =>
  import("./ui/IconShowcase").then(({ IconShowcase }) => ({ default: IconShowcase })),
);

export default definePlugin({
  name: "scopeapp.builtin.icon-gallery",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "icon-gallery",
      title: "workspace.view.title.iconGallery",
      icon: "spark",
      order: 60,
      component: IconGallery,
    });

    registerSettingsPane(ctx, {
      id: BRAND_ICONS_PANE,
      label: "settings.pane.brandIcons",
      group: "advanced",
      icon: "spark",
      order: 110,
      component: IconShowcase,
    });
  },
});
