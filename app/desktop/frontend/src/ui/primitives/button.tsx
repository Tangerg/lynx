import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { forwardRef } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { cn } from "@/lib/classNames";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
};

export const ButtonPrimitive = forwardRef<HTMLButtonElement, ButtonPrimitiveProps>(
  ({ className, type = "button", children, ...props }, ref) => (
    <BaseButton
      {...props}
      ref={ref}
      type={type}
      className={cn(
        "border-0 bg-transparent font-sans text-left focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-45",
        className,
      )}
    >
      {children}
    </BaseButton>
  ),
);

ButtonPrimitive.displayName = "ButtonPrimitive";
