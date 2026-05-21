// Built-in plugin: an icon gallery for @lobehub/icons. Opens as a
// workspace tab so users can browse the available LLM-model / provider /
// application brand icons. Also registers a curated subset in the
// Settings → "Brand icons" pane.

import { IconGallery } from "@/components/icon-gallery/IconGallery";
import { IconShowcase } from "@/components/icon-gallery/IconShowcase";
import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.icon-gallery",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "icon-gallery",
      title: "Icon Gallery",
      icon: "spark",
      openByDefault: false,
      order: 60,
      component: IconGallery,
    });

    host.settings.registerPane({
      id: "brand-icons",
      label: "Brand icons",
      icon: "spark",
      order: 110,
      component: IconShowcase,
    });
  },
});
