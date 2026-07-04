import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

export function Surface({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <div {...props} className={cn("rounded-[14px] bg-surface", className)}>
      {children}
    </div>
  );
}
