// Motion presets — shared easing curves and durations so transitions across
// the app feel like one design system, not a grab bag of values.
//
// The duration on every preset multiplies by `useUiStore.motionScale`
// at read time, so the user's Settings → Motion preference (Off /
// Fast / Default / Slow) ripples through every motion/react animation
// without each call site touching the store. Framer-motion reads
// `transition.duration` on each animate, so a per-access getter is
// fine — no need for hook plumbing at every consumer.

import type { Transition } from "motion/react";
import { useUiStore } from "@/state/uiStore";

// "Sonance" curve — the same cubic-bezier(0.3, 0, 0, 1) we use in CSS,
// tuned for snappy "in" motion that decelerates without overshoot.
const ease = [0.3, 0, 0, 1] as const;

// Build a Transition whose `duration` field is a live getter — reads
// the current motionScale on every access. Framer-motion samples it
// once per animation start, so the cost is negligible and the user
// sees the new scale immediately after toggling.
function scaled(seconds: number): Transition {
  const t = { ease } as Transition;
  Object.defineProperty(t, "duration", {
    enumerable: true,
    get: () => seconds * useUiStore.getState().motionScale,
  });
  return t;
}

export const swift: Transition = scaled(0.22);

// Soft enter from a few px below — for new chat messages.
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: swift,
};
