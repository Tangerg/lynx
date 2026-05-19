import type { ButtonHTMLAttributes, CSSProperties, ReactNode } from "react";
import { cn } from "./cn";

type Variant = "outlined" | "solid" | "accent" | "danger";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: "sm" | "md";
  children: ReactNode;
};

const SIZE_STYLES: Record<NonNullable<Props["size"]>, CSSProperties> = {
  sm: { height: 26, fontSize: 10.5, padding: "0 12px" },
  md: {},
};

// Outlined / solid / accent / danger pill — the project's primary CTA shape.
export function PillButton({
  variant = "outlined", size = "md", className, style, children, ...rest
}: Props) {
  const variantCls = variant === "outlined" ? "" : variant;
  return (
    <button
      {...rest}
      className={cn("pill-btn", variantCls, className)}
      style={{ ...SIZE_STYLES[size], ...style }}
    >
      {children}
    </button>
  );
}
