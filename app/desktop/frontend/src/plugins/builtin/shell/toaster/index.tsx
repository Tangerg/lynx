// Built-in plugin: mounts the toast layer on the "app.overlay" slot.
//
// ctx.notify(...) still dispatches a DOM event, so this plugin just owns
// the listening component. Pulling it out of PluginProvider means the
// provider has zero JSX of its own — pure orchestration.

import { PluginToaster } from "@/plugins/host/PluginToaster";
import { contributeLayout, definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "scopeapp.builtin.toaster",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "toaster",
      // Last so toast portals stay above command UI.
      order: 100,
      component: PluginToaster,
    });
  },
});
