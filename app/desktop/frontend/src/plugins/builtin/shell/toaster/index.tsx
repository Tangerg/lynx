// Built-in plugin: mounts the toast layer on the "app.overlay" slot.
//
// host.notify(...) still dispatches a DOM event, so this plugin just owns
// the listening component. Pulling it out of PluginProvider means the
// provider has zero JSX of its own — pure orchestration.

import { PluginToaster } from "@/plugins/host/PluginToaster";
import { definePlugin } from "@/plugins/sdk";
import { toasterOverlaySlot } from "./application/toasterContributions";

export default definePlugin({
  name: "lyra.builtin.toaster",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", toasterOverlaySlot(PluginToaster));
  },
});
