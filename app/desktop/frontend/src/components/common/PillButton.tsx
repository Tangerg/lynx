import type { VariantProps } from "class-variance-authority";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";

// The project's primary CTA shape — outlined / solid / accent / danger
// variants in two sizes. md (8px) corner per DESIGN.md §6.
const styles = cva(
  "inline-flex items-center gap-1.5 rounded-md font-sans font-medium tracking-normal " +
    "transition-colors duration-150 ease-out " +
    "disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      variant: {
        outlined:
          "border border-line text-fg bg-transparent hover:bg-surface-2 hover:border-line-soft",
        solid: "bg-white text-black border border-white hover:scale-[1.04]",
        accent: "bg-accent text-on-accent border border-accent hover:scale-[1.04]",
        danger: "bg-transparent text-negative border border-negative hover:bg-negative/8",
      },
      size: {
        sm: "h-6.5 px-3 text-[11px]",
        md: "h-8 px-3.5 text-[13px]",
      },
    },
    defaultVariants: { variant: "outlined", size: "md" },
  },
);

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> &
  VariantProps<typeof styles> & {
    children: ReactNode;
  };

export function PillButton({ variant, size, className, children, ...rest }: Props) {
  return (
    <button {...rest} className={cn(styles({ variant, size }), className)}>
      {children}
    </button>
  );
}
