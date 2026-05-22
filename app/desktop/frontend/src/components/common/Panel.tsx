import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  className?: string;
  children: ReactNode;
};

// One of the rounded columns that make up the kernel layout. Callers
// add a marker className ("sidebar", "chat", …) to opt into the
// section-specific styling already in app.css.
export function Panel({ className, children }: Props) {
  return <div className={cn("panel", className)}>{children}</div>;
}
