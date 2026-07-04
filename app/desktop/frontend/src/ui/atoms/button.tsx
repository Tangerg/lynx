import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

const buttonStyles = cva(
  "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap font-sans font-medium leading-none outline-none transition-[background-color,color,scale] duration-[120ms] ease-out disabled:cursor-not-allowed disabled:opacity-45 disabled:active:scale-100",
  {
    variants: {
      variant: {
        ghost: "bg-transparent text-fg-muted hover:bg-fg/[0.06] hover:text-fg",
        soft: "bg-surface-2 text-fg-soft hover:bg-surface-3 hover:text-fg",
        outline: "border border-field bg-transparent text-fg-soft hover:bg-fg/[0.05] hover:text-fg",
        primary: "bg-cta text-cta-text hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative/10",
      },
      size: {
        xs: "h-6 rounded-sm px-2 text-[11.5px]",
        sm: "h-7 rounded-md px-2.5 text-[13px]",
        md: "h-8 rounded-md px-3 text-[13px]",
        "icon-sm": "h-7 w-7 rounded-md p-0",
        "icon-md": "h-8 w-8 rounded-md p-0",
        "icon-lg": "h-10 w-10 rounded-md p-0",
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
