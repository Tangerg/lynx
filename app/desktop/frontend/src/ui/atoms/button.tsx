import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

// Every variant carries a border — transparent where there is nothing to draw.
// Without it a bordered variant is 2px taller and 2px wider than a borderless one
// at the same size, so an outline button and a ghost button never sit on the same
// baseline in a toolbar. The horizontal padding is likewise 1px short of the
// nominal step, compensating for that border so the ink lands where it would have
// without one.
//
// Glyphs ride at 80%: a chrome icon should read a step behind its label. The
// `:not([class*='opacity-'])` guard is the escape hatch — a semantic glyph (a
// warning triangle, a status dot) sets its own opacity and keeps it.
const buttonStyles = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap",
    "border border-transparent font-sans font-medium leading-none outline-none",
    "transition-[background-color,border-color,color,scale] duration-[120ms] ease-out",
    "disabled:cursor-not-allowed disabled:opacity-64 disabled:active:scale-100",
    "[&_svg:not([class*='opacity-'])]:opacity-80",
  ].join(" "),
  {
    variants: {
      variant: {
        ghost: "bg-transparent text-fg-muted hover:bg-hover hover:text-fg",
        soft: "bg-surface-2 text-fg-soft hover:bg-surface-3 hover:text-fg",
        outline: "border-field bg-transparent text-fg-soft hover:bg-hover hover:text-fg",
        primary: "bg-cta text-cta-text hover:bg-cta-hover",
        danger: "bg-transparent text-negative hover:bg-negative/10",
      },
      size: {
        xs: "h-6 rounded-sm px-[7px] text-ui-sm",
        sm: "h-7 rounded-md px-[9px] text-ui-md",
        md: "h-8 rounded-md px-[11px] text-ui-md",
        "icon-xs": "h-6 w-6 rounded-sm p-0",
        "icon-sm": "h-7 w-7 rounded-md p-0",
        "icon-md": "h-8 w-8 rounded-md p-0",
        "icon-lg": "h-10 w-10 rounded-md p-0",
      },
      press: {
        true: "active:scale-[var(--press-scale)]",
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
