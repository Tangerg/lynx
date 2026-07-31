import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

// Tailwind Merge cannot infer custom `--text-*` theme variables from generated
// CSS. Without this list it classifies `text-ui-md` as a colour, then removes a
// preceding `text-cta-text` / `text-fg-soft` as if the font-size and ink
// utilities conflicted. Keep the type ladder named here so size replaces size
// while colour and size survive together.
const mergeTailwindClasses = extendTailwindMerge({
  extend: {
    theme: {
      text: [
        "ui-2xs",
        "ui-xs",
        "ui-sm",
        "ui-md",
        "ui-lg",
        "code",
        "display-sm",
        "display-md",
        "display-lg",
        "display-xl",
      ],
    },
  },
});

/** Compose conditional class names and resolve conflicting Tailwind utilities. */
export function cn(...inputs: ClassValue[]) {
  return mergeTailwindClasses(clsx(inputs));
}
