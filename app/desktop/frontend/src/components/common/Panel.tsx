import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  className?: string;
  children: ReactNode;
};

// Rounded column of the kernel layout. The marker className passed in
// ("sidebar", "chat", …) opts into the section-specific rules in
// layout.css; the base `.panel` class provides the lifted-card chrome.
export function Panel({ className, children }: Props) {
  return <div className={cn("panel", className)}>{children}</div>;
}
