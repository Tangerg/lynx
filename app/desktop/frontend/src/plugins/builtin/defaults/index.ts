// Built-in plugins: starter defaults for config / data / themes / roles /
// title / palette commands. Each one is its own plugin so a third-party
// can swap any single piece without taking out the others.
//
// `defaultCommands` and `defaultData` live in sibling files because they
// are substantially bigger than the rest (the reactive command rebuild,
// and the full data-provider manifest + protocol→sidebar mappers).

import { RUNTIME_BASE } from "@/main/config";
import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { ACCENT, MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";

export { defaultCommands } from "./commands";
export { defaultData } from "./data";

export const defaultConfig = definePlugin({
  name: "lyra.builtin.default-config",
  version: "1.0.0",
  setup({ host }) {
    host.config.set("api.baseUrl", RUNTIME_BASE);
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
    host.extensions.contribute(MESSAGE_ROLE, {
      id: "user",
      displayName: t("role.user"),
      icon: "user",
      avatarVariant: "msg-user",
    });
    host.extensions.contribute(MESSAGE_ROLE, {
      // Neutral fallback only — the live model name (from the session's model
      // via models.list) is resolved in ChatStream and passed to MessageBlock.
      // This shows when no model resolves (e.g. before the lists load).
      id: "assistant",
      displayName: t("role.assistant"),
      icon: "spark",
      avatarVariant: "msg-agent",
    });
    host.extensions.contribute(MESSAGE_ROLE, {
      id: "system",
      displayName: t("role.system"),
      icon: "shield",
      avatarVariant: "msg-agent",
    });
  },
});
