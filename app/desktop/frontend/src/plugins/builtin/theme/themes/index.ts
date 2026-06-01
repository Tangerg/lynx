// Bundle of every built-in theme plugin.
//
// Adding a theme = drop a new folder under plugins/builtin/, register
// it here, done. The manifest (plugins/builtin/index.ts) pulls in this
// single array and never has to touch individual theme imports — the
// `infrastructure` group stays uncluttered as more themes ship.

import type { PluginSpec } from "@/plugins/sdk";
import { definePluginPack } from "@/plugins/sdk";
import customTheme from "../custom-theme";
import atomOneDark from "../atom-one-dark";
import atomOneLight from "../atom-one-light";
import catppuccinLatte from "../catppuccin-latte";
import catppuccinMacchiato from "../catppuccin-macchiato";
import catppuccinMocha from "../catppuccin-mocha";
import lyraDark from "../lyra-dark";
import lyraLight from "../lyra-light";
import solarizedDark from "../solarized-dark";
import solarizedLight from "../solarized-light";
import tokyoNightLight from "../tokyo-night-light";
import tokyoNightStorm from "../tokyo-night-storm";

export const builtinThemes: PluginSpec[] = [
  lyraDark,
  lyraLight,
  atomOneDark,
  atomOneLight,
  tokyoNightStorm,
  tokyoNightLight,
  solarizedDark,
  solarizedLight,
  catppuccinMocha,
  catppuccinMacchiato,
  catppuccinLatte,
];

// All built-in themes shipped as one Plugin Pack — a single manifest entry that
// loads each theme (+ the user-editable custom theme) as an independent child.
// Themes have no `requires` and distinct ids, so child array order is purely
// the picker's sort hint. Demonstrates `definePluginPack` on first-party code.
export const themesPack = definePluginPack({
  name: "lyra.builtin.themes",
  version: "1.0.0",
  children: [...builtinThemes, customTheme],
});
