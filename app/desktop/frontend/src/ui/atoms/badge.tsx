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
const styles = cva("inline-flex shrink-0 items-center gap-1 font-medium", {
  variants: {
    tone: {
      neutral: "bg-surface-2 text-fg-muted",
      accent: "bg-accent-badge text-accent",
      success: "bg-success-badge text-success",
      warning: "bg-warning-badge text-warning",
      negative: "bg-negative-badge text-negative",
      info: "bg-info-badge text-info",
    },
    size: {
      sm: "rounded-sm px-1.5 py-px text-ui-xs",
      md: "rounded-sm px-2 py-0.5 text-ui-sm",
    },
    shape: { square: "", pill: "rounded-pill" },
  },
  defaultVariants: { tone: "neutral", size: "sm", shape: "square" },
});

export type BadgeProps = Omit<VariantProps<typeof styles>, "tone"> & {
  tone?: Tone;
  children: ReactNode;
  className?: string;
  title?: string;
};

export function Badge({ tone, size, shape, className, children, title }: BadgeProps) {
  return (
    <span title={title} className={cn(styles({ tone, size, shape }), className)}>
      {children}
    </span>
  );
}
