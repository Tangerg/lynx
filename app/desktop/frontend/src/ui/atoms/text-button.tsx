import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

// A word that acts, with no box around it: "Import JSON", "3 more", "Edit args".
// Distinct from `Button` rather than a variant of it — Button is a control with
// metrics (a height, an inset, a fill that answers the pointer), and everything
// it puts in the base class is something this has to undo. Here the affordance
// IS the type: the tone lifts toward full ink on hover, and nothing moves.
const styles = cva(
  "inline-flex items-center gap-1.5 bg-transparent p-0 text-left transition-colors " +
    "disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      tone: {
        muted: "text-fg-muted hover:text-fg",
        accent: "text-accent hover:text-accent",
        negative: "text-negative hover:opacity-80",
      },
      size: { sm: "text-ui-sm", md: "text-ui-md" },
    },
    defaultVariants: { tone: "muted", size: "md" },
  },
);

export type TextButtonProps = Omit<ButtonPrimitiveProps, "children"> &
  VariantProps<typeof styles> & { children: ReactNode };

export function TextButton({ tone, size, className, children, ...props }: TextButtonProps) {
  return (
    <ButtonPrimitive {...props} className={cn(styles({ tone, size }), className)}>
      {children}
    </ButtonPrimitive>
  );
}
