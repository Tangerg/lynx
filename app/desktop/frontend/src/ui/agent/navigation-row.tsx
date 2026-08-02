import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";

// The hover swap is the row's own choreography, so the row performs it. Named
// group rather than a bare one because rows nest — a project header wraps a
// button which is itself a group, and an unnamed group would be ambiguous.
// Tailwind only emits utilities it can read as complete literals, so the group
// variants are spelled out rather than interpolated.
const ROW_GROUP = "group/row";

// The resting glyph steps aside — and stops intercepting clicks — so the action
// REPLACES it instead of stacking on top. The row supplies the element this sits
// on: a caller's own glyph may run its own opacity animation (a pulsing dot),
// and a running animation wins over a variant.
const RESTING_GLYPH =
  "transition-opacity group-hover/row:pointer-events-none group-hover/row:opacity-0 group-focus-within/row:pointer-events-none group-focus-within/row:opacity-0";

const HOVER_ACTION =
  "opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100";

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  iconClassName?: string;
  trailing?: ReactNode;
  /**
   * Revealed on hover or focus, taking the resting `trailing` glyph's place. A
   * sibling of the row rather than a child, because a button cannot nest inside
   * one — which is the whole reason this is a prop: positioning it, reserving its
   * space, and cross-fading it are the row's business, and a caller doing that
   * for itself is a caller reimplementing the row.
   */
  action?: ReactNode;
  indent?: "none" | "nested";
  children?: ReactNode;
}

/**
 * A work-index row. Hover and selection share one fill, and selection reads through
 * the background rather than the label's weight — the label itself stays at full
 * ink, because dimming resting nav text is what makes a sidebar look washed out.
 */
export function AgentRow({
  active,
  icon,
  iconClassName,
  trailing,
  action,
  indent = "none",
  className,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  const row = (
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
        "text-fg transition-[background-color,color] duration-[var(--dur-fast)]",
        "hover:bg-hover hover:text-fg focus-visible:bg-hover",
        "data-[active]:bg-selected data-[active]:text-fg",
        indent === "nested" ? "px-2 pl-8" : "px-2",
        action && "pr-8",
        className,
      )}
    >
      {icon && <Icon name={icon} size="sm" className={cn("shrink-0 text-fg/95", iconClassName)} />}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing && <span className={cn(action && RESTING_GLYPH)}>{trailing}</span>}
    </Button>
  );

  if (!action) return row;
  return (
    <div className={cn("relative select-none", ROW_GROUP)}>
      {row}
      <span className={cn("absolute inset-y-0 right-1 grid place-items-center", HOVER_ACTION)}>
        {action}
      </span>
    </div>
  );
}
