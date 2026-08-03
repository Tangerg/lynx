import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";
import { UI_TYPE_STEPS } from "./typography";

// Tailwind Merge cannot infer custom `--text-*` theme variables from generated
// CSS. Without this list it classifies `text-ui-md` as a colour, then removes a
// preceding `text-cta-text` / `text-fg-soft` as if the font-size and ink
// utilities conflicted. Keep the type ladder named here so size replaces size
// while colour and size survive together.
//
// A step missing from this list fails SILENTLY and in two different directions:
// dropped when a colour utility follows it, ignored when another size does. The
// UI steps come from the ladder itself for that reason; `check-design-tokens`
// holds the editorial half, which lives only in globals.css.
const EDITORIAL_STEPS = ["display-sm", "display-md", "display-lg", "display-xl"];

const mergeTailwindClasses = extendTailwindMerge({
  extend: { theme: { text: [...UI_TYPE_STEPS, ...EDITORIAL_STEPS] } },
});

/** Compose conditional class names and resolve conflicting Tailwind utilities. */
export function cn(...inputs: ClassValue[]) {
  return mergeTailwindClasses(clsx(inputs));
}
