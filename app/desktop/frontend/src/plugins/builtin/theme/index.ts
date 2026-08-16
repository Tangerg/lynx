// The appearance package: colour themes, visual styles and the document painter.
//
// Adding a theme = drop a new file under `theme/themes/`, add it to the array
// below, done — mirrors `i18n/` (pack entry + `locales/` data files). The
// manifest pulls in this single pack and never touches individual theme
// imports. `themes/` holds the data files; `kit/` holds the shared
// theme-authoring helper (`defineColorThemePlugin` + tokens + types).

import type { AnyPlugin } from "dougong";
import { appearancePainter } from "./appearancePainter";
import customTheme from "./themes/custom-theme";
import atomOneDark from "./themes/atom-one-dark";
import atomOneLight from "./themes/atom-one-light";
import catppuccinLatte from "./themes/catppuccin-latte";
import catppuccinMocha from "./themes/catppuccin-mocha";
import lyraDark from "./themes/lyra-dark";
import lyraLight from "./themes/lyra-light";
import solarizedDark from "./themes/solarized-dark";
import solarizedLight from "./themes/solarized-light";
import tokyoNightLight from "./themes/tokyo-night-light";
import tokyoNightStorm from "./themes/tokyo-night-storm";
import { builtinVisualStyles } from "./visualStyles";

const builtinThemes: AnyPlugin[] = [
  lyraDark,
  lyraLight,
  atomOneDark,
  atomOneLight,
  tokyoNightStorm,
  tokyoNightLight,
  solarizedDark,
  solarizedLight,
  catppuccinMocha,
  catppuccinLatte,
];

export const appearancePlugins: AnyPlugin[] = [
  ...builtinThemes,
  customTheme,
  ...builtinVisualStyles,
  appearancePainter,
];
