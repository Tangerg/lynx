// Built-in plugin: mounts the toast layer on the "app.overlay" slot.
//
// ctx.notify(...) still dispatches a DOM event, so this plugin just owns
// the listening component. Pulling it out of PluginProvider means the
// provider has zero JSX of its own — pure orchestration.

import { PluginToaster } from "@/plugins/host/PluginToaster";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { toasterOverlaySlot } from "./application/toasterContributions";

export default definePlugin({
  name: "lyra.builtin.toaster",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", toasterOverlaySlot(PluginToaster));
  },
});
