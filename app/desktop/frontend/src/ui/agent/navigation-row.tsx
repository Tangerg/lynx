import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";
import { Tooltip } from "@/ui/atoms/tooltip";
import { AgentOverflowLabel } from "./overflow-label";

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
  /**
   * A second line under the label — state, time, counts. Opting in swaps the
   * row's fixed height for a minimum, which is why it is a prop and not
   * something a caller can hand in through `children`: the row height is part of
   * the index's rhythm and a caller that grew its own would break the column.
   */
  detail?: ReactNode;
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
  /** Reveal a string label on row hover/focus when its rendered width overflows. */
  revealOverflow?: boolean;
  children?: ReactNode;
}

/**
 * A work-index row. Hover and selection share one fill, and selection reads through
 * the background rather than the label's weight. The atom defaults to full ink;
 * callers may lower quiet leaf rows while keeping hierarchy and interaction here.
 */
export function AgentRow({
  active,
  icon,
  iconClassName,
  detail,
  trailing,
  action,
  indent = "none",
  revealOverflow = false,
  className,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  const overflowText = revealOverflow && typeof children === "string" ? children : undefined;
  const button = (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      press={false}
      data-active={active ? "" : undefined}
      className={cn(
        // A step up in SIZE from the chrome default, and only size. The index is
        // the one column a user reads standing still, and at 13px a Latin label
        // filled about a third of its row's height — the row looked airy because
        // the type had abandoned it, not because it was generous. The weight came
        // along for that ride and did not belong on it: at 500 every row in the
        // column is emphasised, so nothing in it is, and the open row is left
        // with only its fill to say so. The reference sets the whole index at the
        // body weight for exactly that reason, and `Button` defaults to medium,
        // so this has to say `normal` out loud.
        "agent-row w-full justify-start rounded-[var(--row-radius)] text-left text-ui-md font-normal",
        "gap-[var(--density-row-gap)]",
        "text-fg transition-[background-color,color] duration-[var(--dur-color)]",
        "hover:bg-hover hover:text-fg focus-visible:bg-hover",
        // Selection reads through the FILL alone, which is what this atom always
        // claimed and never did: it also bumped the weight, so the resting rows
        // were the light ones and the whole column's rhythm hung off which row
        // happened to be open.
        "data-[active]:bg-selected data-[active]:text-fg",
        // `h-auto` is load-bearing: the size variant ships a fixed `h-`, and
        // without replacing it the second line renders outside the row's fill.
        detail
          ? "h-auto min-h-[var(--density-row-height)] items-start py-2"
          : "h-[var(--density-row-height)]",
        // A nested row's label lands on its parent's label, not near it: the
        // parent's own inset, its glyph, and the gap after it. Spelled as the
        // sum so a density or icon-ladder change moves both together — the
        // literal it replaced put children two pixels LEFT of their parent.
        indent === "nested"
          ? "px-2 pl-[calc(0.5rem+var(--icon-sm)+var(--density-row-gap))]"
          : "px-2",
        action && "pr-8",
        className,
      )}
    >
      {icon && (
        <Icon
          name={icon}
          size="sm"
          className={cn("shrink-0 text-fg", detail && "mt-px", iconClassName)}
        />
      )}
      {/* A two-line row breathes on `body` rather than `snug`: stacked lines set
          at heading leading read as one clipped block, and the index is the
          surface a user scans longest. */}
      <span className="flex min-w-0 flex-1 flex-col gap-px">
        <span
          className={cn(
            "flex min-w-0 items-center gap-2",
            detail ? "leading-body" : "leading-snug",
          )}
        >
          {overflowText ? (
            <AgentOverflowLabel text={overflowText} />
          ) : (
            <span className="min-w-0 flex-1 truncate-fade">{children}</span>
          )}
          {trailing && <span className={cn("shrink-0", action && RESTING_GLYPH)}>{trailing}</span>}
        </span>
        {detail != null && (
          <span className="min-w-0 truncate-fade text-ui-2xs leading-body text-fg-faint">
            {detail}
          </span>
        )}
      </span>
    </Button>
  );
  const row = overflowText ? (
    <Tooltip label={overflowText} side="right" sideOffset={8} delayDuration={500}>
      {button}
    </Tooltip>
  ) : (
    button
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
