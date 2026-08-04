import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import type { Tone } from "@/lib/tone";
import { cn } from "@/lib/classNames";

// A word carrying a state: "done", "safe", "needs auth", "3 errors".
//
// One component because it was fourteen: every card that needed a tinted chip
// paired a fill with an ink by hand, and they disagreed — negative alone was
// spelled at five alphas across the tree. The tone picks the pair; the caller
// picks the words.
// Fully round, always. A word carrying a state is the one thing here that is not a
// container, and rounding it completely is how it stops reading as a small one — the
// reference sheets agree on this even where they disagree about every other radius
// (Nova is half pills; JetBrains keeps its status chips round inside a 6px world).
// Horizontal padding is a step wider than a square chip would need, because the
// curve eats the first and last few pixels of the inset.
const styles = cva("inline-flex shrink-0 items-center gap-1 rounded-pill font-medium", {
  variants: {
    // The FILL carries the hue; the word is ink. Tone-on-its-own-tone was the
    // original shape and it does not survive measurement: a tone ink on an 18% tint
    // of itself runs 4.23–4.48:1 in light and 2.67–4.44:1 in dark, against the 4.5:1
    // AA floor — the accent and info chips in dark mode were 2.67. Both sides move
    // together when the fill is made of the ink, so no amount of tuning fixes it,
    // and the user's own accent can be any colour at all.
    //
    // `fg-soft` clears 5.9–7.6:1 on every one of these fills in both schemes and is
    // independent of the accent, because the hue is not doing the legibility work.
    // It never was: the fill already said which category this is, and the reference
    // sheet's own chips set their word in a near-black grey for the same reason.
    tone: {
      neutral: "bg-surface-2 text-fg-muted",
      accent: "bg-accent-badge text-fg-soft",
      success: "bg-success-badge text-fg-soft",
      warning: "bg-warning-badge text-fg-soft",
      negative: "bg-negative-badge text-fg-soft",
      info: "bg-info-badge text-fg-soft",
    },
    size: {
      sm: "px-2 py-px text-ui-xs",
      md: "px-2.5 py-0.5 text-ui-sm",
    },
  },
  defaultVariants: { tone: "neutral", size: "sm" },
});

export type BadgeProps = Omit<VariantProps<typeof styles>, "tone"> & {
  tone?: Tone;
  children: ReactNode;
  className?: string;
  title?: string;
};

export function Badge({ tone, size, className, children, title }: BadgeProps) {
  return (
    <span title={title} className={cn(styles({ tone, size }), className)}>
      {children}
    </span>
  );
}
