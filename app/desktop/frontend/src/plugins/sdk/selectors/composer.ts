// Composer selectors — status bar items, modes, attachment sources,
// placeholders, key bindings.

import type {
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
} from "../types";
import {
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_MODE,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
} from "../kernelPoints";
import { usePluginStore } from "../registry";
import { lookupExtensionPoint, useExtensionPoint } from "./extensions";

export function useComposerStatus(): ComposerStatusSpec[] {
  return useExtensionPoint(COMPOSER_STATUS);
}

export function useComposerModes(): ComposerModeSpec[] {
  return useExtensionPoint(COMPOSER_MODE);
}

export function useComposerAttachmentSources(): ComposerAttachmentSourceSpec[] {
  return useExtensionPoint(COMPOSER_ATTACHMENT_SOURCE);
}

/**
 * Pick one composer placeholder via weighted random. Returns undefined
 * when nothing's registered; callers should fall back to a sensible
 * default. Pure read — call once at component mount, not on every render.
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

/** Look up a composer key binding by canonical combo. */
export function lookupComposerKeyBinding(canonical: string): ComposerKeyBindingSpec | undefined {
  return usePluginStore.getState().composerKeyBindings.get(canonical)?.value;
}
