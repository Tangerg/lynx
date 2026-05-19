// Built-in plugin: registers HTTP-backed fetchers for the 8 standard data
// query keys (sessions, projects, files-changed, diff, terminal, grep,
// file-head, mcp-servers).
//
// Other plugins can override any of these — last-write-wins replaces the
// fetcher, so a "fixture-data" plugin loaded after this one can swap in
// recorded JSON for tests / offline mode.

import { api } from "@/lib/http";
import { definePlugin } from "@/plugins/sdk";

// HTTP_KEYS maps query keys to backend paths. Every key from queries.ts
// has an entry here — adding a key in queries.ts without one will make
// that hook reject at runtime.
const HTTP_KEYS = [
  "sessions",
  "projects",
  "files-changed",
  "diff",
  "terminal",
  "grep",
  "file-head",
  "mcp-servers",
] as const;

export default definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  setup({ host }) {
    for (const key of HTTP_KEYS) {
      host.data.registerProvider({
        key,
        fetcher: () => api.get(key).json(),
      });
    }
  },
});
