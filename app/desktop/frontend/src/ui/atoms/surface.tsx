import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

export function Surface({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "rounded-[12px] bg-surface shadow-[inset_0_0_0_0.5px_var(--color-field)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
