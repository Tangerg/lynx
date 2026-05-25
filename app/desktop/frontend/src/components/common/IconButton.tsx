import type {VariantProps} from "class-variance-authority";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cva  } from "class-variance-authority";
import { cn } from "@/lib/utils";

// Icon-only button with three variants used across the app.
//   ghost        — header / chat-tab actions (32px circle, transparent until hover)
//   rail         — collapsed sidebar items (40px rounded square)
//   rail-primary — emphasized rail item (subtle bg in idle state)
const styles = cva(
  "grid place-items-center text-fg-muted cursor-pointer border-0 bg-transparent " +
    "transition-colors duration-150 ease-out hover:text-fg disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      variant: {
        ghost: "h-8 w-8 rounded-full hover:bg-surface-2",
        rail: "h-10 w-10 rounded-lg hover:bg-surface",
        "rail-primary": "h-10 w-10 rounded-lg bg-surface-2 text-fg hover:bg-surface-3",
      },
      active: {
        true: "",
        false: "",
      },
    },
    compoundVariants: [
      // Active state varies per variant — ghost goes accent-colored,
      // rail variants stay neutral (the user signals active via other UI).
      { variant: "ghost", active: true, class: "text-accent" },
    ],
    defaultVariants: { variant: "ghost", active: false },
  },
);

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> &
  VariantProps<typeof styles> & {
    children: ReactNode;
  };

export function IconButton({ variant, active, className, children, ...rest }: Props) {
  // Icon-only buttons need an accessible name. If the caller supplied
  // `title` (the hover tooltip) but didn't override `aria-label`, mirror
  // it so screen readers get the same text the sighted user sees.
  const ariaLabel = rest["aria-label"] ?? rest.title;
  return (
    <button {...rest} aria-label={ariaLabel} className={cn(styles({ variant, active }), className)}>
      {children}
    </button>
  );
}
