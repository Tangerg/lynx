import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "./button";
import { Tooltip } from "./tooltip";

interface IconButtonProps extends Omit<ButtonProps, "children" | "variant" | "size"> {
  icon: IconName;
  /** xs = 24px, for inline affordances inside a row of text; sm = 28px, dense
   *  chrome rows; md = 32px, the default chrome control; lg = 40px, for a
   *  standalone touch target inside content. */
  size?: "xs" | "sm" | "md" | "lg";
  iconSize?: number;
  /** Toggled-on: a pinned item, an open panel. Reads as pressed in, not hovered. */
  active?: boolean;
  /** A step fainter at rest. For buttons that sit *inside* content — a list row,
   *  a card, a message bubble — where the glyph must not compete with what the
   *  user came to read. Chrome buttons (headers, toolbars) leave it off: there
   *  they are the primary affordance. */
  quiet?: boolean;
  /** Hover and focus help. Doubles as the accessible name unless `aria-label`
   *  overrides it — an icon has no text of its own, so the two are the same fact
   *  and callers should not have to state it twice. */
  title?: string;
  /** Unread / running count, drawn as a pip on the top-right corner. Omitted
   *  when zero or empty: a badge showing "0" is noise wearing an alert's colour. */
  badge?: string | number;
}

const BOX = { xs: "icon-xs", sm: "icon-sm", md: "icon-md", lg: "icon-lg" } as const;
const ICON_SIZE = { xs: 12, sm: 14, md: 16, lg: 16 } as const;

// Glyph-only chrome button. The app Tooltip carries the help rather than the
// native `title` attribute: 250ms instead of the OS's ~1s, and it appears on
// keyboard focus, which the native one never does.
export function IconButton({
  icon,
  size = "md",
  iconSize = ICON_SIZE[size],
  active,
  quiet,
  badge,
  className,
  title,
  ...props
}: IconButtonProps) {
  return (
    <Tooltip label={title}>
      <Button
        {...props}
        aria-label={props["aria-label"] ?? title}
        variant="ghost"
        size={BOX[size]}
        data-active={active ? "" : undefined}
        className={cn(
          "relative data-[active]:bg-selected data-[active]:text-fg",
          quiet && "text-fg-faint",
          className,
        )}
      >
        <Icon name={icon} size={iconSize} strokeWidth={1.8} />
        {badge !== undefined && badge !== "" && badge !== 0 && (
          <span className="absolute -top-0.5 -right-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-accent px-0.5 font-mono text-ui-2xs font-semibold text-on-accent">
            {badge}
          </span>
        )}
      </Button>
    </Tooltip>
  );
}
