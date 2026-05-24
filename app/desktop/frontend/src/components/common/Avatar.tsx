import type { ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

// Small circular avatar with role-aware coloring. The `variant` prop
// owns the full visual treatment via Tailwind utilities — consumers
// no longer need to wrap with a per-role parent class.
const avatarStyles = cva(
  "grid place-items-center rounded-full font-semibold shrink-0 select-none",
  {
    variants: {
      variant: {
        "msg-agent":  "bg-accent text-on-accent",
        "msg-user":   "bg-surface-3 text-fg",
        "msg-system": "bg-transparent border border-line text-fg-muted",
        "user-card":  "bg-surface-3 text-fg",
      },
      size: {
        sm: "h-7 w-7 text-[11px]",
        md: "h-8 w-8 text-[12px]",
        lg: "h-9 w-9 text-[13px]",
      },
    },
    defaultVariants: { size: "md" },
  },
);

type Props = VariantProps<typeof avatarStyles> & {
  variant: NonNullable<VariantProps<typeof avatarStyles>["variant"]>;
  children: ReactNode;
  className?: string;
};

export function Avatar({ variant, size, children, className }: Props) {
  return <div className={cn(avatarStyles({ variant, size }), className)}>{children}</div>;
}
