// Built-in plugin: sets the baseline document title to "Lyra".
//
// A future plugin can change it (e.g. "(3) Lyra — fern-api" when there
// are unread notifications and a project is active). Latest setter wins,
// so user plugins simply override.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.default-title",
  version: "1.0.0",
  setup({ host }) {
    host.window.setTitle("Lyra");
  },
});
