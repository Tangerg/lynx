import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "./Icon";
import { MENU_ITEM_CLASSES, MenuItem } from "./Menu";

// A context-menu row: leading 12px icon + truncating label. `destructive`
// paints it in the negative tone for delete-style actions. Pairs with
// MENU_CONTENT_CLASSES on the enclosing ContextMenu.Content.
export function MenuIconItem({
  icon,
  onSelect,
  destructive,
  children,
}: {
  icon: IconName;
  onSelect: () => void;
  destructive?: boolean;
  children: ReactNode;
}) {
  return (
    <MenuItem
      onClick={onSelect}
      className={cn(
        MENU_ITEM_CLASSES,
        "grid-cols-[14px_minmax(0,1fr)]",
        destructive &&
          "text-negative data-[highlighted]:bg-negative/10 data-[highlighted]:text-negative",
      )}
    >
      <Icon name={icon} size={12} />
      <span className="truncate">{children}</span>
    </MenuItem>
  );
}
