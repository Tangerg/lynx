// Motion presets — shared easing curves and durations so transitions across
// the app feel like one design system, not a grab bag of values.
//
// The duration on every preset multiplies by the published motion scale at read
// time, so the user's Settings → Motion preference (Off / Fast / Default / Slow)
// ripples through every motion/react animation without each call site touching
// it. Framer-motion reads `transition.duration` on each animate, so a per-access
// getter is fine — no need for hook plumbing at every consumer.

import type { Transition } from "motion/react";
import { motionScale, visualStyleMotion } from "./appearance";

// Build a Transition whose `duration` field is a live getter — reads the
// current scale on every access. Framer-motion samples it once per animation
// start, so the cost is negligible and the user sees the new scale immediately
// after toggling.
function scaled(duration: "mediumMs" | "disclosureMs"): Transition {
  const t = {} as Transition;
  Object.defineProperty(t, "duration", {
    enumerable: true,
    get: () => (visualStyleMotion()[duration] / 1000) * motionScale(),
  });
  Object.defineProperty(t, "ease", {
    enumerable: true,
    get: () => visualStyleMotion().easeOut,
  });
  return t;
}

/** Expand/collapse and banner replacement: enough time for structure to read. */
export const disclosureTransition: Transition = scaled("disclosureMs");

/** Small content entrance: shorter than structural disclosure motion. */
const contentEnterTransition: Transition = scaled("mediumMs");

// Soft enter from a few px below — for new chat messages.
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: contentEnterTransition,
};
