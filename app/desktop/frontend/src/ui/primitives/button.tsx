import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { cn } from "@/lib/classNames";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
  ref?: Ref<HTMLButtonElement>;
};

export function ButtonPrimitive({
  className,
  type = "button",
  children,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  return (
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
  );
}
