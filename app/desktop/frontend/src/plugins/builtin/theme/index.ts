// The `theme` package entry: every built-in theme shipped as one Plugin Pack.
//
// Adding a theme = drop a new folder under `theme/`, add it to the array below,
// done. The manifest (plugins/builtin/index.ts) pulls in this single pack and
// never touches individual theme imports. The `kit/` subfolder holds the
// shared theme-authoring helper (`defineThemePlugin` + tokens + types).

import type { PluginSpec } from "@/plugins/sdk";
import { definePluginPack } from "@/plugins/sdk";
import customTheme from "./custom-theme";
import atomOneDark from "./atom-one-dark";
import atomOneLight from "./atom-one-light";
import catppuccinLatte from "./catppuccin-latte";
import catppuccinMacchiato from "./catppuccin-macchiato";
import catppuccinMocha from "./catppuccin-mocha";
import lyraDark from "./lyra-dark";
import lyraLight from "./lyra-light";
import solarizedDark from "./solarized-dark";
import solarizedLight from "./solarized-light";
import tokyoNightLight from "./tokyo-night-light";
import tokyoNightStorm from "./tokyo-night-storm";

const builtinThemes: PluginSpec[] = [
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

// Themes have no `requires` and distinct ids, so child array order is purely
// the picker's sort hint. The user-editable custom theme rides along as the
// last child.
export const themesPack = definePluginPack({
  name: "lyra.builtin.themes",
  version: "1.0.0",
  children: [...builtinThemes, customTheme],
});
