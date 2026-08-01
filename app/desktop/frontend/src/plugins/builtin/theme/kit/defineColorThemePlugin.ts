// Helper for the "colour theme as plugin" pattern — turns a typed ColorThemePluginSpec
// into a PluginSpec ready for the builtin manifest. Required sections
// (brand / surfaces / ink / borders / semantic) are enforced by TypeScript;
// depthStep / cta / extras are optional palette overrides.
//
// The token-computation workhorse (buildTokenMap + default ladders) lives
// in `./tokens.ts` so it can be unit-tested in isolation. The type
// surface (ColorThemePluginSpec + sections) lives in `./types.ts` so
// `tokens.ts` can pull it without forming a cycle with this file.

import type { PluginSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { colorThemeContribution } from "./colorThemeContribution";
import type { ColorThemePluginSpec } from "./types";

export type {
  ThemeBorders,
  ThemeBrand,
  ThemeCta,
  ThemeInk,
  ColorThemePluginSpec,
  ThemeSemantic,
  ThemeSurfaces,
} from "./types";

export function defineColorThemePlugin(spec: ColorThemePluginSpec): PluginSpec {
  const theme = colorThemeContribution(spec);
  return definePlugin({
    name: `lyra.builtin.color-theme-${spec.id}`,
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(COLOR_THEME, theme);
    },
  });
}
