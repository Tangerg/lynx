import type { ComponentProps, ReactNode } from "react";
import { ContextMenu as BaseContextMenu } from "@base-ui/react/context-menu";
import { Menu as BaseMenu } from "@base-ui/react/menu";
import { cn } from "@/lib/utils";

export const MENU_CONTENT_CLASSES =
  "z-50 overflow-hidden rounded-md border-0 bg-surface p-1 shadow-[var(--shadow-popover)] animate-rise-in";

export const MENU_ITEM_CLASSES =
  "grid items-center gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg";

const MENU_SEPARATOR_CLASSES = "mx-1 my-1 h-px bg-line-soft/40";

type DropdownPositionerProps = ComponentProps<typeof BaseMenu.Positioner>;
type DropdownPopupProps = ComponentProps<typeof BaseMenu.Popup>;
type ContextPopupProps = ComponentProps<typeof BaseContextMenu.Popup>;

interface FloatingContentProps {
  children: ReactNode;
  className?: string;
  positionerClassName?: string;
  side?: DropdownPositionerProps["side"];
  align?: DropdownPositionerProps["align"];
  sideOffset?: DropdownPositionerProps["sideOffset"];
  alignOffset?: DropdownPositionerProps["alignOffset"];
}

type DropdownContentProps = FloatingContentProps &
  Omit<DropdownPopupProps, keyof FloatingContentProps | "className">;

type ContextContentProps = FloatingContentProps &
  Omit<ContextPopupProps, keyof FloatingContentProps | "className">;

function DropdownContent({
  children,
  className,
  positionerClassName,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: DropdownContentProps) {
  return (
    <BaseMenu.Portal>
      <BaseMenu.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        className={positionerClassName}
      >
        <BaseMenu.Popup {...popupProps} className={cn(MENU_CONTENT_CLASSES, className)}>
          {children}
        </BaseMenu.Popup>
      </BaseMenu.Positioner>
    </BaseMenu.Portal>
  );
}

function ContextContent({
  children,
  className,
  positionerClassName,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: ContextContentProps) {
  return (
    <BaseContextMenu.Portal>
      <BaseContextMenu.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        className={positionerClassName}
      >
        <BaseContextMenu.Popup {...popupProps} className={cn(MENU_CONTENT_CLASSES, className)}>
          {children}
        </BaseContextMenu.Popup>
      </BaseContextMenu.Positioner>
    </BaseContextMenu.Portal>
  );
}

function DropdownSeparator({ className, ...props }: ComponentProps<typeof BaseMenu.Separator>) {
  return <BaseMenu.Separator {...props} className={cn(MENU_SEPARATOR_CLASSES, className)} />;
}

function ContextSeparator({
  className,
  ...props
}: ComponentProps<typeof BaseContextMenu.Separator>) {
  return <BaseContextMenu.Separator {...props} className={cn(MENU_SEPARATOR_CLASSES, className)} />;
}

export const MenuItem = BaseMenu.Item;

export const DropdownMenu = {
  Root: BaseMenu.Root,
  Trigger: BaseMenu.Trigger,
  Content: DropdownContent,
  Item: BaseMenu.Item,
  Separator: DropdownSeparator,
  SubmenuRoot: BaseMenu.SubmenuRoot,
  SubmenuTrigger: BaseMenu.SubmenuTrigger,
} as const;

export const ContextMenu = {
  Root: BaseContextMenu.Root,
  Trigger: BaseContextMenu.Trigger,
  Content: ContextContent,
  Item: BaseContextMenu.Item,
  Separator: ContextSeparator,
  SubmenuRoot: BaseContextMenu.SubmenuRoot,
  SubmenuTrigger: BaseContextMenu.SubmenuTrigger,
} as const;
