import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { useActiveSessionId, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { resolveComposerModelSelection } from "../application/modelSelection";
import { useComposerModelPreference } from "./modelPreference";

/** The model the next run will use: composerStore's provider+model pair
 *  resolved against the live model list, then the active durable Session's
 *  exact provider/model selection before the first explicit pick, then the catalog default when no
 *  Session is active. While an active Session summary is loading there is no
 *  fallback: choosing early would turn a query race into a model override.
 *  `undefined` when no provider is enabled yet.
 *
 *  One home for "which model is selected" so the surfaces that gate on its
 *  exact input modalities — the toolbar attach button and the composer's
 *  paste/drop image staging — can't disagree. */
export function useSelectedModelSelection() {
  const { data: models = [] } = useModels();
  const preference = useComposerModelPreference();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const activeSessionSelection = activeSessionId
    ? sessions === undefined
      ? undefined
      : (() => {
          const session = sessions.find((candidate) => candidate.id === activeSessionId);
          return session
            ? {
                provider: session.provider,
                model: session.model,
                reasoningEffort: session.reasoningEffort,
              }
            : null;
        })()
    : null;
  return resolveComposerModelSelection(models, preference, activeSessionSelection);
}

export function useSelectedModel() {
  return useSelectedModelSelection()?.model;
}
