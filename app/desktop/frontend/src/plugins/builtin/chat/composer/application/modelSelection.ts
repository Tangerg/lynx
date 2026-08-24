import type { ComposerModelPreference } from "./ports/state";

export interface ComposerModelOption {
  id: string;
  provider: string;
}

export interface ComposerSessionModelSelection {
  provider: string;
  model: string;
}

/**
 * Resolve the model shown by the Composer.
 *
 * A deliberate in-process preference wins across sessions. Before one exists,
 * an already-durable Session owns the default for its next Run; only the
 * no-session welcome surface falls back to the first enabled catalog entry.
 * `undefined` means the active Session summary is still resolving, so callers
 * must not race that read by materializing the catalog fallback as an override.
 */
export function resolveComposerModel<T extends ComposerModelOption>(
  models: readonly T[],
  preference: ComposerModelPreference,
  activeSessionSelection: ComposerSessionModelSelection | null | undefined,
): T | undefined {
  const preferred = models.find(
    (candidate) => candidate.provider === preference.provider && candidate.id === preference.model,
  );
  if (preferred) return preferred;
  if (activeSessionSelection === undefined) return undefined;
  if (activeSessionSelection !== null) {
    const sessionModel = models.find(
      (candidate) =>
        candidate.provider === activeSessionSelection.provider &&
        candidate.id === activeSessionSelection.model,
    );
    if (sessionModel) return sessionModel;
  }
  return models[0];
}

/** Only a deliberate Composer preference becomes a Run override. A model
 * derived from the active Session stays Session-owned and is intentionally
 * omitted so Runtime reads that same durable pair at admission. */
export function resolveComposerRunOptions(preference: ComposerModelPreference): {
  provider?: string;
  model?: string;
} {
  return preference.provider && preference.model
    ? { provider: preference.provider, model: preference.model }
    : {};
}
