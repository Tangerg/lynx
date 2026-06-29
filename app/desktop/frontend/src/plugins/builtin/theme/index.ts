// The `theme` package entry: every built-in theme shipped as one Plugin Pack.
//
// Adding a theme = drop a new file under `theme/themes/`, add it to the array
// below, done — mirrors `i18n/` (pack entry + `locales/` data files). The
// manifest pulls in this single pack and never touches individual theme
// imports. `themes/` holds the data files; `kit/` holds the shared
// theme-authoring helper (`defineThemePlugin` + tokens + types).

import type { PluginSpec } from "@/plugins/sdk";
import { definePluginPack } from "@/plugins/sdk";
import customTheme from "./themes/custom-theme";
import atomOneDark from "./themes/atom-one-dark";
import atomOneLight from "./themes/atom-one-light";
import catppuccinLatte from "./themes/catppuccin-latte";
import catppuccinMacchiato from "./themes/catppuccin-macchiato";
import catppuccinMocha from "./themes/catppuccin-mocha";
import lyraDark from "./themes/lyra-dark";
import lyraLight from "./themes/lyra-light";
import premiumDark from "./themes/premium-dark";
import solarizedDark from "./themes/solarized-dark";
import solarizedLight from "./themes/solarized-light";
import tokyoNightLight from "./themes/tokyo-night-light";
import tokyoNightStorm from "./themes/tokyo-night-storm";

const builtinThemes: PluginSpec[] = [
  lyraDark,
  lyraLight,
  premiumDark,
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
