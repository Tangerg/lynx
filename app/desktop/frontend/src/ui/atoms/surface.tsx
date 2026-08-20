import type { ComponentPropsWithRef } from "react";
import { cva } from "class-variance-authority";
import type { VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";

// A card: the raised panel a settings section, a form or an inline chat card sits
// on. It owns the inset as well as the fill, because a card without one is a
// rectangle — eleven callsites wrote `rounded-lg bg-surface p-4` by hand while
// this component existed and did the first two thirds of it.
const styles = cva(
  "rounded-[var(--surface-card-radius)] bg-[var(--app-card-surface)] shadow-[var(--shadow-surface-card)]",
  {
    variants: {
      // `none` is for a card that hosts its own rows, each with their own inset —
      // the appearance pane stacks sections that already pad themselves.
      inset: { none: "", sm: "p-3", md: "p-4" },
    },
    defaultVariants: { inset: "md" },
  },
);

export type SurfaceProps = ComponentPropsWithRef<"div"> & VariantProps<typeof styles>;

export function Surface({ inset, className, children, ...props }: SurfaceProps) {
  return (
    <div {...props} className={cn(styles({ inset }), className)}>
      {children}
    </div>
  );
}
