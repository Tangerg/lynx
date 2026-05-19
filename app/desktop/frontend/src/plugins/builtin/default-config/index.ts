// Built-in plugin: seeds the shared `host.config` store with the baseline
// values the shell expects. Today that's just the AG-UI backend base URL;
// future entries (default model, retry policy, etc.) live here too.
//
// Plugins that want to *override* a value can register at a later position
// in the manifest and call `host.config.set(...)` again — config is
// last-writer-wins.

import { AGUI_BASE } from "@/lib/http";
import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.default-config",
  version: "1.0.0",
  setup({ host }) {
    host.config.set("api.baseUrl", AGUI_BASE);
  },
});
