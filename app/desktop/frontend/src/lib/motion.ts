// Motion presets — shared easing curves and durations so transitions across
// the app feel like one design system, not a grab bag of values.

import type { Transition } from "motion/react";

// "Sonance" curve — the same cubic-bezier(0.3, 0, 0, 1) we use in CSS, tuned
// for snappy "in" motion that decelerates without overshoot.
export const ease = [0.3, 0, 0, 1] as const;

export const fast: Transition  = { duration: 0.16, ease };
export const swift: Transition = { duration: 0.22, ease };

// Spring used for inline expansion (tool card preview, reasoning body) —
// just enough bounce to feel responsive without overshooting on small heights.
export const inlineSpring: Transition = {
  type: "spring",
  stiffness: 420,
  damping: 36,
  mass: 0.7,
};

// Soft enter from a few px below — for new chat messages.
export const enterUp = {
  initial: { opacity: 0, y: 6 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
  transition: swift,
};

// Scale-in for floating panels (command palette).
export const popIn = {
  initial: { opacity: 0, scale: 0.96, y: 4 },
  animate: { opacity: 1, scale: 1, y: 0 },
  exit: { opacity: 0, scale: 0.97, y: 2 },
  transition: fast,
};
