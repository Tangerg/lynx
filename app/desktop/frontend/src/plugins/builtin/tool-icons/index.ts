// Built-in plugin: maps tool-fn names → icon glyphs.
//
// The hardcoded fallback inside `toolIconFor` mirrors these registrations
// so first paint (before plugins load) is still sensible. This plugin is
// what allows third-party tools to add their own icons.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.tool-icons",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerIcon("read_file",  "file");
    host.tool.registerIcon("write_file", "file");
    host.tool.registerIcon("edit_file",  "file");
    host.tool.registerIcon("grep",       "search");
    host.tool.registerIcon("bash",       "terminal");
    host.tool.registerIcon("web_search", "globe");
  },
});
