import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

export function Kbd({ className, ...props }: ComponentPropsWithoutRef<"kbd">) {
  return (
    <kbd
      {...props}
      className={cn(
        "pointer-events-none inline-flex h-5 min-w-5 select-none items-center justify-center rounded-2xs bg-sunken px-1 font-sans text-ui-sm font-medium leading-none text-fg-muted",
        className,
      )}
    />
  );
}
