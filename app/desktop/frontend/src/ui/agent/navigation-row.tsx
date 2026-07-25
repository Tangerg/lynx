import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";

/**
 * Marker a row wrapper must carry for the reveal classes below to fire. Named
 * rather than a bare `group` because rows nest — a project header wraps its own
 * button, which is itself a group — and an unnamed group would be ambiguous.
 */
export const AGENT_ROW_GROUP = "group/row";

/**
 * Fade a resting glyph out — and stop it intercepting clicks — the moment its row
 * reveals hover actions, so the actions REPLACE it instead of stacking on top.
 * Apply to a static element: if the element animates its own opacity (a pulsing
 * dot, say) the running animation wins, so put this on a wrapper instead.
 *
 * Tailwind only emits utilities it can read as complete literals, so the group
 * variants are spelled out rather than interpolated.
 */
export const AGENT_ROW_RESTING_GLYPH =
  "transition-opacity group-hover/row:pointer-events-none group-hover/row:opacity-0 group-focus-within/row:pointer-events-none group-focus-within/row:opacity-0";

/** The hover toolbar that takes the resting glyph's slot. */
export const AGENT_ROW_HOVER_ACTION =
  "opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100";

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  iconClassName?: string;
  trailing?: ReactNode;
  indent?: "none" | "nested";
  children?: ReactNode;
}

/**
 * A work-index row. Hover and selection share one fill: selection reads through
 * ink strength rather than a heavier background, so a long list stays quiet and
 * the accent colour stays reserved for live state.
 */
export function AgentRow({
  active,
  icon,
  iconClassName,
  trailing,
  indent = "none",
  className,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  return (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      press={false}
      data-active={active ? "" : undefined}
      className={cn(
        "h-[var(--density-row-height)] w-full justify-start rounded-sm text-left text-ui-md font-normal",
        "gap-[var(--density-row-gap)]",
        "text-fg/89 transition-[background-color,color] duration-100",
        "hover:bg-fg/[0.04] hover:text-fg focus-visible:bg-fg/[0.04]",
        "data-[active]:bg-fg/[0.04] data-[active]:text-fg",
        indent === "nested" ? "px-2 pl-8" : "px-2",
        className,
      )}
    >
      {icon && (
        <Icon
          name={icon}
          size={14}
          strokeWidth={1.8}
          className={cn("shrink-0 text-fg/95", iconClassName)}
        />
      )}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing}
    </Button>
  );
}
