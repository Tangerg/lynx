import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

export function Surface({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <div {...props} className={cn("rounded-lg bg-surface", className)}>
      {children}
    </div>
  );
}
