import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "./cn";

type Variant = "ghost" | "rail" | "rail-primary";

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> & {
  variant?: Variant;
  active?: boolean;
  children: ReactNode;
};

// 32 / 40 px square button used for header actions and rail icons. The visual
// variant picks the right CSS hook from app.css.
export function IconButton({
  variant = "ghost", active, className, children, ...rest
}: Props) {
  const cls =
    variant === "rail"         ? "rail-btn" :
    variant === "rail-primary" ? "rail-btn primary" :
    "icon-btn";
  return (
    <button {...rest} className={cn(cls, active && "active", className)}>
      {children}
    </button>
  );
}
