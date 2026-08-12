import type { ComposerModelPreference } from "./ports/state";

export interface ComposerModelOption {
  id: string;
  provider: string;
}

/**
 * Resolve the model shown and seeded by the Composer.
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
  activeSessionModel: string | null | undefined,
): T | undefined {
  const preferred = models.find(
    (candidate) => candidate.provider === preference.provider && candidate.id === preference.model,
  );
  if (preferred) return preferred;
  if (activeSessionModel === undefined) return undefined;
  if (activeSessionModel !== null) {
    const sessionModel = models.find((candidate) => candidate.id === activeSessionModel);
    if (sessionModel) return sessionModel;
  }
  return models[0];
}
