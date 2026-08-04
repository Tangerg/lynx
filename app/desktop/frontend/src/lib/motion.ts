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
function scaled(duration: "fastMs" | "mediumMs" | "disclosureMs"): Transition {
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

/**
 * A selection travelling from one place to another — the segmented control's chip.
 *
 * The only kind of motion CSS genuinely cannot express: the lifted chip is a fill on
 * whichever segment is active, and a transition animates a property WITHIN one
 * element, never between two. So a chip either appeared where it landed or you built
 * a separate absolutely-positioned indicator and measured its offsets by hand, which
 * is what `layoutId` does correctly and for free.
 */
export const selectionTransition: Transition = scaled("fastMs");

/**
 * Something the user just added or took away — an attachment chip, a paste.
 *
 * Scale from just under, not a slide: a chip has no direction to come from, and the
 * exit matters more than the entrance. Without one, removing an attachment made the
 * chips after it jump left in a single frame with nothing to say the one you clicked
 * had been the thing that left.
 *
 * Presence ONLY. Pair it with `layout` and the survivors would slide into the gap
 * instead of jumping — for the price of a measurement on every render of whatever
 * holds them, and the composer re-renders on every keystroke. A nicety on the rare
 * interaction is not worth a cost on the constant one.
 */
export const chipPresence = {
  initial: { opacity: 0, scale: 0.92 },
  animate: { opacity: 1, scale: 1 },
  exit: { opacity: 0, scale: 0.92 },
  transition: selectionTransition,
};

// Soft enter from a few px below — for new chat messages.
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: contentEnterTransition,
};
