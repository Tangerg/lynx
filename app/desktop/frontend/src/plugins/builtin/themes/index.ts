// Bundle of every built-in theme plugin.
//
// Adding a theme = drop a new folder under plugins/builtin/, register
// it here, done. The manifest (plugins/builtin/index.ts) pulls in this
// single array and never has to touch individual theme imports — the
// `infrastructure` group stays uncluttered as more themes ship.

import lyraDark from "../lyra-dark";
import lyraLight from "../lyra-light";
import atomOneDark from "../atom-one-dark";
import atomOneLight from "../atom-one-light";
import tokyoNightStorm from "../tokyo-night-storm";
import tokyoNightLight from "../tokyo-night-light";
import solarizedDark from "../solarized-dark";
import solarizedLight from "../solarized-light";
import catppuccinMocha from "../catppuccin-mocha";
import catppuccinLatte from "../catppuccin-latte";
import type { PluginSpec } from "@/plugins/sdk";

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
  catppuccinLatte,
];
