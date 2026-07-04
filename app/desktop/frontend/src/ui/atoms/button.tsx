import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

const buttonStyles = cva(
  "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap font-sans font-medium leading-none outline-none transition-[background-color,color,box-shadow,scale] duration-[120ms] ease-out disabled:cursor-not-allowed disabled:opacity-45 disabled:active:scale-100",
  {
    variants: {
      variant: {
        ghost: "bg-transparent text-fg-muted hover:bg-fg/[0.045] hover:text-fg",
        soft: "bg-surface-2 text-fg-soft shadow-[inset_0_0_0_0.5px_var(--color-field)] hover:bg-surface-3 hover:text-fg",
        outline:
          "bg-transparent text-fg-muted shadow-[inset_0_0_0_0.5px_var(--color-field)] hover:bg-fg/[0.045] hover:text-fg",
        primary: "bg-cta text-on-cta shadow-[var(--shadow-border)] hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative/10",
      },
      size: {
        xs: "h-6 rounded-[7px] px-2 text-[11.5px]",
        sm: "h-7 rounded-[8px] px-2.5 text-[12px]",
        md: "h-8 rounded-[8px] px-3 text-[12.5px]",
        "icon-sm": "h-7 w-7 rounded-[8px] p-0",
        "icon-md": "h-8 w-8 rounded-[8px] p-0",
        "icon-lg": "h-10 w-10 rounded-[9px] p-0",
      },
      press: {
        true: "active:scale-[0.96]",
        false: "",
      },
    },
    defaultVariants: {
      variant: "ghost",
      size: "md",
      press: true,
    },
  },
);

export type ButtonProps = Omit<ButtonPrimitiveProps, "children"> &
  VariantProps<typeof buttonStyles> & {
    children?: ReactNode;
  };

export function Button({ variant, size, press, className, children, ...props }: ButtonProps) {
  return (
    <ButtonPrimitive {...props} className={cn(buttonStyles({ variant, size, press }), className)}>
      {children}
    </ButtonPrimitive>
  );
}
