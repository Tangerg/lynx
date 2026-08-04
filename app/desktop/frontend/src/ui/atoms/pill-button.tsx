import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

// The project's primary CTA shape — outlined / solid / accent / danger
// variants in two sizes. These controls sit inside dense toolbars, so they
// stay compact while using softer native-pill corners.
const styles = cva(
  "inline-flex items-center gap-1.5 rounded-pill font-sans font-medium tracking-normal " +
    "transition-[background-color,color,scale] duration-[var(--dur-fast)] ease-out active:scale-[var(--press-scale)] " +
    "disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      variant: {
        outlined: "border-[0.5px] border-field text-fg-soft hover:bg-hover hover:text-fg",
        solid: "bg-cta text-cta-text hover:bg-cta-hover",
        accent: "bg-cta text-cta-text",
        danger:
          "bg-transparent text-negative border-[0.5px] border-negative hover:bg-negative-wash",
      },
      size: {
        sm: "h-6.5 px-3 text-ui-sm",
        md: "h-8 px-3.5 text-ui-md",
      },
    },
    defaultVariants: { variant: "outlined", size: "md" },
  },
);

type Props = Omit<ButtonPrimitiveProps, "children"> &
  VariantProps<typeof styles> & {
    children: ReactNode;
  };

export function PillButton({ variant, size, className, children, ...rest }: Props) {
  return (
    <ButtonPrimitive {...rest} className={cn(styles({ variant, size }), className)}>
      {children}
    </ButtonPrimitive>
  );
}
