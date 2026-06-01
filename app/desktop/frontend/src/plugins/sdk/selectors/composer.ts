// Composer placeholder picker — the one composer read with real logic. Plain
// reads (status / modes / attachment sources / key binding) go through the
// generic substrate: `useExtensionPoint(COMPOSER_MODE)` etc.

import type { ComposerPlaceholderSpec } from "../types";
import { COMPOSER_PLACEHOLDER } from "../kernelPoints";
import { lookupExtensionPoint } from "./extensions";

/**
 * Pick one composer placeholder via weighted random. Returns undefined when
 * nothing's registered; callers fall back to a default. Pure read — call once
 * at mount, not every render.
 */
export function pickComposerPlaceholder(): ComposerPlaceholderSpec | undefined {
  const specs = lookupExtensionPoint(COMPOSER_PLACEHOLDER);
  if (specs.length === 0) return undefined;
  const total = specs.reduce((sum, s) => sum + (s.weight ?? 1), 0);
  if (total <= 0) return undefined;
  let r = Math.random() * total;
  for (const spec of specs) {
    r -= spec.weight ?? 1;
    if (r <= 0) return spec;
  }
  return specs.at(-1);
}
