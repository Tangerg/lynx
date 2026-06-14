import { useModels } from "@/lib/data/queries";
import { useComposerStore } from "@/state/composerStore";

/** The model the next run will use: composerStore's provider+model pair
 *  resolved against the live model list, falling back to the first model while
 *  the pair is still null (boot) or no longer matches an enabled provider.
 *  `undefined` when no provider is enabled yet.
 *
 *  One home for "which model is selected" so the surfaces that gate on its
 *  `multimodal` capability — the toolbar attach button and the composer's
 *  paste/drop image staging — can't disagree. */
export function useSelectedModel() {
  const { data: models = [] } = useModels();
  const provider = useComposerStore((s) => s.provider);
  const model = useComposerStore((s) => s.model);
  return models.find((m) => m.provider === provider && m.id === model) ?? models[0];
}
