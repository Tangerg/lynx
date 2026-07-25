import { cn } from "@/lib/utils";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "./button";
import { Tooltip } from "./tooltip";

interface IconButtonProps extends Omit<ButtonProps, "children" | "variant" | "size"> {
  icon: IconName;
  /** sm = 28px, for dense chrome rows; md = 32px, the default chrome control. */
  size?: "sm" | "md";
  iconSize?: number;
  /** Toggled-on: a pinned item, an open panel. Reads as pressed in, not hovered. */
  active?: boolean;
  /** Hover and focus help. Doubles as the accessible name unless `aria-label`
   *  overrides it — an icon has no text of its own, so the two are the same fact
   *  and callers should not have to state it twice. */
  title?: string;
}

// Glyph-only chrome button. The app Tooltip carries the help rather than the
// native `title` attribute: 250ms instead of the OS's ~1s, and it appears on
// keyboard focus, which the native one never does.
export function IconButton({
  icon,
  size = "md",
  iconSize = size === "sm" ? 14 : 16,
  active,
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
        size={size === "sm" ? "icon-sm" : "icon-md"}
        data-active={active ? "" : undefined}
        className={cn("data-[active]:bg-fg/[0.06] data-[active]:text-fg", className)}
      >
        <Icon name={icon} size={iconSize} strokeWidth={1.8} />
      </Button>
    </Tooltip>
  );
}
