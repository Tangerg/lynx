import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { cn } from "@/lib/utils";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
};

export function ButtonPrimitive({
  className,
  type = "button",
  children,
  ...props
}: ButtonPrimitiveProps) {
  return (
    <BaseButton
      {...props}
      type={type}
      className={cn(
        "border-0 bg-transparent font-sans text-left focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-45",
        className,
      )}
    >
      {children}
    </BaseButton>
  );
}
