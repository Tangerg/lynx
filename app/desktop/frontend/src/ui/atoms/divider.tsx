import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

/**
 * A label with a rule beside it — the only shape in this system that draws a line
 * inside content. A day boundary, a compaction marker, a settled decision.
 *
 * `align` decides whether the rule flanks the label or trails it. Both shapes
 * existed as hand-rolled `<span className="h-px flex-1 …" />` pairs at three
 * callsites, at three different ink alphas, because the centred variant lived here
 * and the left-aligned one had nowhere to be.
 *
 * The rule is `bg-divider`; one palette token owns its ink so callers never choose
 * local alphas.
 *
 * `intent` only tunes the icon chip's ink. The label always takes `fg-faint`: this
 * shape marks a boundary in the reading, and a boundary that competes with the
 * reading is a heading.
 */
export function Divider({
  icon,
  intent = "neutral",
  align = "center",
  className,
  children,
}: {
  icon?: ReactNode;
  intent?: "neutral" | "accent";
  align?: "center" | "start";
  className?: string;
  children: ReactNode;
}) {
  const rule = <span aria-hidden className="h-px flex-1 bg-divider" />;
  return (
    <div
      className={cn("my-2 flex items-center gap-3 text-ui-sm font-medium text-fg-faint", className)}
    >
      {align === "center" && rule}
      {icon && (
        <div
          className={cn(
            "grid h-4.5 w-4.5 place-items-center rounded-full bg-surface-2",
            intent === "accent" ? "text-accent" : "text-fg-faint",
          )}
        >
          {icon}
        </div>
      )}
      <span className="min-w-0 shrink-0">{children}</span>
      {rule}
    </div>
  );
}
