// Composer selectors — status bar items, modes, attachment sources,
// placeholders, key bindings.

import type {
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
} from "../types";
import { usePluginStore } from "../registry";
import { mapOwned, useSortedList } from "./_helpers";

export function useComposerStatus(): ComposerStatusSpec[] {
  return useSortedList(usePluginStore((s) => s.composerStatus));
}

export function useComposerModes(): ComposerModeSpec[] {
  return useSortedList(usePluginStore((s) => s.composerModes));
}

export function useComposerAttachmentSources(): ComposerAttachmentSourceSpec[] {
  return useSortedList(usePluginStore((s) => s.composerAttachmentSources));
}

/**
 * Pick one composer placeholder via weighted random. Returns undefined
 * when nothing's registered; callers should fall back to a sensible
 * default. Pure read — call once at component mount, not on every render.
 */
export function pickComposerPlaceholder(): ComposerPlaceholderSpec | undefined {
  const specs = mapOwned(usePluginStore.getState().composerPlaceholders);
  if (specs.length === 0) return undefined;
  const total = specs.reduce((sum, s) => sum + (s.weight ?? 1), 0);
  if (total <= 0) return undefined;
  let r = Math.random() * total;
  for (const spec of specs) {
    r -= spec.weight ?? 1;
    if (r <= 0) return spec;
  }
  return specs[specs.length - 1];
}

/** Look up a composer key binding by canonical combo. */
export function lookupComposerKeyBinding(canonical: string): ComposerKeyBindingSpec | undefined {
  return usePluginStore.getState().composerKeyBindings.get(canonical)?.value;
}
