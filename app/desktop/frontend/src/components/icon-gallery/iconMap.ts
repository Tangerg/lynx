// Shared loader for @lobehub/icons brand glyphs.
//
// We deliberately bypass the package barrel (`@lobehub/icons`) because it
// re-exports `features/*` modules that depend on `@lobehub/ui` + `antd`,
// neither of which we ship. Instead we glob just the per-brand `Mono.js`
// files (only react + jsx-runtime).

import type { ComponentType } from "react";
import { toc as rawToc } from "@lobehub/icons/es/toc";

const monoModules = import.meta.glob<{ default: ComponentType<{ size?: number }> }>(
  "../../../node_modules/@lobehub/icons/es/*/components/Mono.js",
  { eager: true },
);

export const IconMap: Record<string, ComponentType<{ size?: number }>> = {};
for (const [path, mod] of Object.entries(monoModules)) {
  const match = path.match(/\/es\/([^/]+)\/components\/Mono\.js$/);
  // match[1] is the captured group — defined when match is non-null.
  if (match) IconMap[match[1]!] = mod.default;
}

export { rawToc };
export type TocEntry = (typeof rawToc)[number];

// Quick lookup: id -> toc metadata.
export const TocById: Record<string, TocEntry> = {};
for (const e of rawToc) TocById[e.id] = e;
