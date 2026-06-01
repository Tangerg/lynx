// Built-in plugins: starter defaults for config / data / themes / roles /
// title / palette commands. Each one is its own plugin so a third-party
// can swap any single piece without taking out the others.
//
// `defaultCommands` lives in a sibling file because it's substantially
// bigger than the rest (~100 lines for the reactive command rebuild).

import type {
  MCPServer as SidebarMCPServer,
  SidebarProject,
  SidebarSession,
} from "@/lib/data/queries";
import type { MCPServer as RpcMCPServer, Session } from "@/rpc";
import { api } from "@/lib/data/http";
import { AGUI_BASE } from "@/main/config";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { ACCENT } from "@/plugins/sdk/kernelPoints";

// Cutover mappers: several side-panel keys now ride the JSON-RPC stack
// instead of REST GET. Where the protocol shape differs from the sidebar
// row, we map it down here (the protocol intentionally omits client-side
// presentation like the MCP icon — see API.md §6.5).

// `sessions` — protocol Session is richer than the sidebar row.
function toSidebarSession(s: Session): SidebarSession {
  return {
    id: s.id,
    title: s.title,
    status: s.status,
    model: s.model,
    time: s.updatedAt || s.createdAt,
  };
}

// `mcp-servers` — the protocol MCPServer carries no id/icon (both are
// client-side). Use the MCP name as the stable id and map name → icon.
const MCP_ICON: Record<string, string> = {
  Filesystem: "folder",
  Git: "branch",
  Shell: "terminal",
  "Web Search": "globe",
  Linear: "list",
  GitHub: "git",
  Postgres: "tool",
  Slack: "chat",
};
function toSidebarMCPServer(s: RpcMCPServer): SidebarMCPServer {
  return {
    id: s.name,
    name: s.name,
    desc: s.desc,
    tools: s.toolCount,
    status: s.status,
    icon: MCP_ICON[s.name] ?? "tool",
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
    host.extensions.contribute(ACCENT, {
      id: "green",
      label: "Green",
      dark: "#1ed760",
      light: "#169c46",
      order: 0,
    });
    host.extensions.contribute(ACCENT, {
      id: "blue",
      label: "Blue",
      dark: "#82cfff",
      light: "#2563eb",
      order: 1,
    });
    host.extensions.contribute(ACCENT, {
      id: "pink",
      label: "Pink",
      dark: "#e07acc",
      light: "#a823a3",
      order: 2,
    });
    host.extensions.contribute(ACCENT, {
      id: "orange",
      label: "Orange",
      dark: "#ffa42b",
      // Amber (hue ~32), not the old #c2410c which read as red on light bg.
      light: "#d97706",
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

// HTTP_KEYS lists the query keys still served over REST GET. Migrated to
// the JSON-RPC stack so far: sessions, projects, files-changed, mcp-servers
// (registered separately below). The four that remain need view-param
// plumbing (diff / grep / file-head take a path/query the param-less
// provider model can't carry yet) or are a stream (terminal → #160).
// Adding a key in queries without a provider here makes that hook reject.
const HTTP_KEYS = ["diff", "terminal", "grep", "file-head"] as const;

export const defaultData = definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  setup({ host }) {
    const methods = () => getContainer().methods();

    host.data.registerProvider<SidebarSession[]>({
      key: "sessions",
      fetcher: async () => (await methods().sessions.list()).items.map(toSidebarSession),
    });
    host.data.registerProvider<SidebarProject[]>({
      key: "projects",
      fetcher: () => methods().workspace.projects(),
    });
    host.data.registerProvider({
      key: "files-changed",
      fetcher: () => methods().workspace.filesChanged(),
    });
    host.data.registerProvider<SidebarMCPServer[]>({
      key: "mcp-servers",
      fetcher: async () => (await methods().workspace.mcp.list()).map(toSidebarMCPServer),
    });

    for (const key of HTTP_KEYS) {
      host.data.registerProvider({
        key,
        fetcher: () => api.get(key).json(),
      });
    }
  },
});
