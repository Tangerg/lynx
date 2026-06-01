// Helper for the "theme as plugin" pattern — turns a typed ThemePluginSpec
// into a PluginSpec ready for the builtin manifest. Required sections
// (brand / surfaces / ink / borders / semantic) are enforced by TypeScript;
// shadows / radii / depthStep / cta / extras are optional overrides.
//
// The token-computation workhorse (buildTokenMap + default ladders) lives
// in `./tokens.ts` so it can be unit-tested in isolation. The type
// surface (ThemePluginSpec + sections) lives in `./types.ts` so
// `tokens.ts` can pull it without forming a cycle with this file.

import type { PluginSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { THEME } from "@/plugins/sdk/kernelPoints";
import { SCHEME_ICON, buildTokenMap } from "./tokens";
import type { ThemePluginSpec } from "./types";

export type {
  ThemeBorders,
  ThemeBrand,
  ThemeCta,
  ThemeInk,
  ThemePluginSpec,
  ThemeRadii,
  ThemeSemantic,
  ThemeShadows,
  ThemeSurfaces,
} from "./types";

export function defineThemePlugin(spec: ThemePluginSpec): PluginSpec {
  const tokens = buildTokenMap(spec);
  return definePlugin({
    name: `lyra.builtin.theme-${spec.id}`,
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(THEME, {
        id: spec.id,
        label: spec.label,
        scheme: spec.scheme,
        icon: spec.icon ?? SCHEME_ICON[spec.scheme],
        order: spec.order,
        tokens,
      });
    },
  });
}
