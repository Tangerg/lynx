import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Variant =
  | "msg-agent"   // chat message avatar, agent side (handled by .msg.agent rule)
  | "msg-user"    // chat message avatar, user side (handled by .msg.user rule)
  | "user-card";  // sidebar user-card avatar

type Props = {
  variant: Variant;
  children: ReactNode;
  className?: string;
};

// Small circular avatar. Picks the right pre-existing CSS class so we don't
// invent a parallel naming scheme.
export function Avatar({ variant, children, className }: Props) {
  const cls = variant === "user-card" ? "user-avatar" : "msg-avatar";
  return <div className={cn(cls, className)}>{children}</div>;
}
