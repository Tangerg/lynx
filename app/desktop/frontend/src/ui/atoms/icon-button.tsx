import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

// Icon-only button with three variants used across the app.
//   ghost        — header / chat-tab actions (32px circle, transparent until hover)
//   rail         — collapsed sidebar items (40px rounded square)
//   rail-primary — emphasized rail item (subtle bg in idle state)
const styles = cva(
  "grid place-items-center text-fg-muted border-0 bg-transparent " +
    "transition-[background-color,color,scale] duration-[120ms] ease-out hover:text-fg active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-50 disabled:active:scale-100",
  {
    variants: {
      variant: {
        ghost: "h-8 w-8 rounded-md hover:bg-fg/[0.08]",
        rail: "h-10 w-10 rounded-md hover:bg-fg/[0.08]",
        "rail-primary": "h-10 w-10 rounded-md bg-fg/[0.06] text-fg hover:bg-fg/[0.09]",
      },
      active: {
        true: "",
        false: "",
      },
    },
    compoundVariants: [
      // Active state varies per variant — ghost gets the neutral pressed
      // fill, rail variants stay flat (the user signals active via other UI).
      { variant: "ghost", active: true, class: "bg-fg/[0.06] text-fg" },
    ],
    defaultVariants: { variant: "ghost", active: false },
  },
);

type Props = Omit<ButtonPrimitiveProps, "children"> &
  VariantProps<typeof styles> & {
    children: ReactNode;
  };

export function IconButton({ variant, active, className, children, title, ...rest }: Props) {
  // Icon-only buttons need an accessible name. If the caller supplied
  // `title` (the hover tooltip) but didn't override `aria-label`, mirror
  // it so screen readers get the same text the sighted user sees.
  const ariaLabel = rest["aria-label"] ?? title;
  // The native `title` attribute is intentionally dropped — app Tooltip
  // Tooltip handles the hover affordance with a 250ms delay vs the
  // OS-default ~1s lag, and it works on focus too (the native title
  // doesn't).
  return (
    <Tooltip label={title}>
      <ButtonPrimitive
        {...rest}
        aria-label={ariaLabel}
        className={cn(styles({ variant, active }), className)}
      >
        {children}
      </ButtonPrimitive>
    </Tooltip>
  );
}
