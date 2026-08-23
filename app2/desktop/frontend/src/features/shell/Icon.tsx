import type { ComponentPropsWithoutRef } from "react";
import type { LucideIcon } from "lucide-react";

type IconSize = "xs" | "sm" | "md" | "lg";

interface IconProps extends Omit<ComponentPropsWithoutRef<"svg">, "ref"> {
  glyph: LucideIcon;
  size?: IconSize;
}

/**
 * The single icon boundary for Desktop chrome.
 *
 * Product surfaces choose semantics; this primitive owns optical size, stroke,
 * and the fact that decorative glyphs never enter the accessibility tree.
 */
export function Icon({
  glyph: Glyph,
  size = "md",
  className,
  ...props
}: IconProps) {
  return (
    <Glyph
      {...props}
      className={["ui-icon", className].filter(Boolean).join(" ")}
      data-size={size}
      aria-hidden="true"
      focusable="false"
      strokeWidth={1.75}
    />
  );
}
