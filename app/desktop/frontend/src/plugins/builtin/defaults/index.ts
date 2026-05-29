// Built-in plugins: starter defaults for config / data / themes / roles /
// title / palette commands. Each one is its own plugin so a third-party
// can swap any single piece without taking out the others.
//
// `defaultCommands` lives in a sibling file because it's substantially
// bigger than the rest (~100 lines for the reactive command rebuild).

import type { SidebarSession } from "@/lib/data/queries";
import type { Session } from "@/rpc";
import { api } from "@/lib/data/http";
import { AGUI_BASE } from "@/main/config";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";

// First cutover slice: `sessions` rides the JSON-RPC stack
// (methods.sessions.list) instead of REST GET /sessions. The protocol
// `Session` is richer than the sidebar row, so map it down here — the
// rest of the keys below still go through the ky data path.
function toSidebarSession(s: Session): SidebarSession {
  return {
    id: s.id,
    title: s.title,
    status: s.status,
    model: s.model,
    time: s.updatedAt || s.createdAt,
  };
}

export { defaultCommands } from "./commands";

export const defaultConfig = definePlugin({
  name: "lyra.builtin.default-config",
  version: "1.0.0",
  setup({ host }) {
    host.config.set("api.baseUrl", AGUI_BASE);
  },
});

export const defaultTitle = definePlugin({
  name: "lyra.builtin.default-title",
  version: "1.0.0",
  setup({ host }) {
    host.window.setTitle("Lyra");
  },
});

// Themes themselves live in their own plugin folders (lyra-dark/,
// lyra-light/, atom-one-*/, tokyo-night-*/, solarized-*/, catppuccin-*/)
// using the `defineThemePlugin` helper. This plugin only owns the
// accent palette — the 4 colored dots in the Appearance pane.
export const defaultAccents = definePlugin({
  name: "lyra.builtin.default-accents",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerAccent({
      id: "green",
      label: "Green",
      dark: "#1ed760",
      light: "#169c46",
      order: 0,
    });
    host.theme.registerAccent({
      id: "blue",
      label: "Blue",
      dark: "#82cfff",
      light: "#2563eb",
      order: 1,
    });
    host.theme.registerAccent({
      id: "pink",
      label: "Pink",
      dark: "#e07acc",
      light: "#a823a3",
      order: 2,
    });
    host.theme.registerAccent({
      id: "orange",
      label: "Orange",
      dark: "#ffa42b",
      light: "#c2410c",
      order: 3,
    });
  },
});

export const defaultRoles = definePlugin({
  name: "lyra.builtin.default-roles",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerRole({
      id: "user",
      displayName: "You",
      icon: "user",
      avatarVariant: "msg-user",
    });
    host.message.registerRole({
      id: "assistant",
      displayName: "Sonnet 4.5",
      icon: "spark",
      avatarVariant: "msg-agent",
    });
    host.message.registerRole({
      id: "system",
      displayName: "System",
      icon: "shield",
      avatarVariant: "msg-agent",
    });
  },
});

// HTTP_KEYS lists the query keys still served over REST GET. `sessions`
// has moved to the JSON-RPC stack (registered separately below); adding a
// key in queries without a provider here makes that hook reject at runtime.
const HTTP_KEYS = [
  "projects",
  "files-changed",
  "diff",
  "terminal",
  "grep",
  "file-head",
  "mcp-servers",
] as const;

export const defaultData = definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  setup({ host }) {
    host.data.registerProvider<SidebarSession[]>({
      key: "sessions",
      fetcher: async () => {
        const page = await getContainer().methods().sessions.list();
        return page.items.map(toSidebarSession);
      },
    });
    for (const key of HTTP_KEYS) {
      host.data.registerProvider({
        key,
        fetcher: () => api.get(key).json(),
      });
    }
  },
});
