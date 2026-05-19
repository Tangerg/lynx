// Built-in plugin: registers the dark/light themes + the four built-in
// accents (green / blue / pink / orange). Before this plugin existed the
// data was duplicated between SettingsPopover, the Appearance pane, and
// uiStore — now those all read from the registry instead.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.default-themes",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({ id: "dark",  label: "Dark",  scheme: "dark",  icon: "moon", order: 0 });
    host.theme.registerTheme({ id: "light", label: "Light", scheme: "light", icon: "sun",  order: 1 });

    host.theme.registerAccent({ id: "green",  label: "Green",  dark: "#1ed760", light: "#169c46", order: 0 });
    host.theme.registerAccent({ id: "blue",   label: "Blue",   dark: "#82cfff", light: "#2563eb", order: 1 });
    host.theme.registerAccent({ id: "pink",   label: "Pink",   dark: "#e07acc", light: "#a823a3", order: 2 });
    host.theme.registerAccent({ id: "orange", label: "Orange", dark: "#ffa42b", light: "#c2410c", order: 3 });
  },
});
