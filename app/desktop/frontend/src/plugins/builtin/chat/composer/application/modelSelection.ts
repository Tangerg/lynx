import type { ComposerModelPreference } from "./ports/state";

export interface ComposerModelOption {
  id: string;
  provider: string;
  reasoningLevelOrDefault(level?: string | null): string | undefined;
}

export interface ComposerSessionModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface ResolvedComposerModelSelection<T extends ComposerModelOption> {
  model: T;
  reasoningEffort?: string;
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
export function resolveComposerModelSelection<T extends ComposerModelOption>(
  models: readonly T[],
  preference: ComposerModelPreference,
  activeSessionSelection: ComposerSessionModelSelection | null | undefined,
): ResolvedComposerModelSelection<T> | undefined {
  if (preference.kind === "explicit") {
    const preferred = models.find(
      (candidate) =>
        candidate.provider === preference.provider && candidate.id === preference.model,
    );
    if (preferred) {
      return {
        model: preferred,
        reasoningEffort: preferred.reasoningLevelOrDefault(preference.reasoningEffort),
      };
    }
  }
  if (activeSessionSelection === undefined) return undefined;
  if (activeSessionSelection !== null) {
    const sessionModel = models.find(
      (candidate) =>
        candidate.provider === activeSessionSelection.provider &&
        candidate.id === activeSessionSelection.model,
    );
    if (sessionModel) {
      return {
        model: sessionModel,
        // A durable Session selection is an exact historical fact. Preserve an
        // effort the current catalog no longer advertises so the UI does not
        // silently claim Runtime will execute a different intensity; admission
        // can then report the stale selection honestly.
        reasoningEffort:
          activeSessionSelection.reasoningEffort ?? sessionModel.reasoningLevelOrDefault(),
      };
    }
  }
  const fallback = models[0];
  return fallback
    ? { model: fallback, reasoningEffort: fallback.reasoningLevelOrDefault() }
    : undefined;
}

/** Only a deliberate Composer preference becomes a Run override. A model
 * derived from the active Session stays Session-owned and is intentionally
 * omitted so Runtime reads that same durable pair at admission. */
export function resolveComposerRunOptions(preference: ComposerModelPreference): {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
} {
  return preference.kind === "explicit"
    ? {
        provider: preference.provider,
        model: preference.model,
        ...(preference.reasoningEffort ? { reasoningEffort: preference.reasoningEffort } : {}),
      }
    : {};
}
