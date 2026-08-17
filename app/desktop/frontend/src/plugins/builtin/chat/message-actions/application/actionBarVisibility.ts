// Materialization + visibility state machine for the per-message action bar.
//
// A pure selector over item-local material, root attention, and position — no store
// reads, no hover state:
// hover stays in CSS (`group-hover`) so it costs zero re-renders, and this
// only decides the *baseline* the hover layers onto. Modelled on
// assistant-ui's useActionBarFloatStatus (hidden / floating / normal), named
// for what each state means here:
//   - "absent"  → not mounted and not in flow (this turn can still grow)
//   - "hidden"  → never shown (a run is streaming; actions would be noise)
//   - "hover"   → revealed on hover / focus-within (any settled, non-last turn)
//   - "pinned"  → always shown (the last turn — its actions are the primary
//                 next move, so they don't hide-and-seek)

export type MessageActionMaterialization = "active" | "settled";
export type MessageActionsVisibility = "absent" | "hidden" | "hover" | "pinned";

export interface MessageActionsVisibilityInput {
  /** Item-local material is authoritative when root attention crosses a recovery boundary. */
  materialization: MessageActionMaterialization;
  /** A run is streaming — hide every message's actions until it settles. */
  isRunning: boolean;
  /** This is the last message in the thread — pin its actions open. */
  isLast: boolean;
}

export function messageActionsVisibility({
  materialization,
  isRunning,
  isLast,
}: MessageActionsVisibilityInput): MessageActionsVisibility {
  if (materialization === "active") return "absent";
  if (isRunning) return "hidden";
  if (isLast) return "pinned";
  return "hover";
}
