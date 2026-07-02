import type { ComponentProps, ReactNode } from "react";
import { ContextMenu as BaseContextMenu } from "@base-ui/react/context-menu";
import { Menu as BaseMenu } from "@base-ui/react/menu";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "./Icon";

const MENU_CONTENT_CLASSES =
  "z-50 overflow-hidden rounded-md border-[0.5px] border-field bg-surface p-1 shadow-[var(--shadow-popover)] animate-rise-in";

const MENU_ITEM_CLASSES =
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

type DropdownItemProps = ComponentProps<typeof BaseMenu.Item>;
type DropdownSubmenuTriggerProps = ComponentProps<typeof BaseMenu.SubmenuTrigger>;
type ContextItemProps = ComponentProps<typeof BaseContextMenu.Item>;
type ContextSubmenuTriggerProps = ComponentProps<typeof BaseContextMenu.SubmenuTrigger>;

interface ContextIconItemProps extends Omit<ContextItemProps, "children" | "onClick" | "onSelect"> {
  icon: IconName;
  onSelect: () => void;
  destructive?: boolean;
  children: ReactNode;
}

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
        /* Explicit z-index on the portaled positioner itself — the popup's own
           z-50 sits inside this fixed-positioned node, so without a z-index here
           the whole menu stacks by DOM order and loses to a page element that
           owns a stacking context (e.g. the composer's `relative z-[2]` when the
           model picker opens upward). z-50 keeps every menu above app chrome. */
        className={cn("z-50", positionerClassName)}
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
        className={cn("z-50", positionerClassName)}
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

function DropdownItem({ className, ...props }: DropdownItemProps) {
  return <BaseMenu.Item {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function DropdownSubmenuTrigger({ className, ...props }: DropdownSubmenuTriggerProps) {
  return <BaseMenu.SubmenuTrigger {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function ContextItem({ className, ...props }: ContextItemProps) {
  return <BaseContextMenu.Item {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function ContextSubmenuTrigger({ className, ...props }: ContextSubmenuTriggerProps) {
  return <BaseContextMenu.SubmenuTrigger {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function ContextIconItem({
  icon,
  onSelect,
  destructive,
  children,
  className,
  ...props
}: ContextIconItemProps) {
  return (
    <ContextItem
      {...props}
      onClick={onSelect}
      className={cn(
        "grid-cols-[14px_minmax(0,1fr)]",
        destructive &&
          "text-negative data-[highlighted]:bg-negative/10 data-[highlighted]:text-negative",
        className,
      )}
    >
      <Icon name={icon} size={12} />
      <span className="truncate">{children}</span>
    </ContextItem>
  );
}

export const DropdownMenu = {
  Root: BaseMenu.Root,
  Trigger: BaseMenu.Trigger,
  Content: DropdownContent,
  Item: DropdownItem,
  Separator: DropdownSeparator,
  SubmenuRoot: BaseMenu.SubmenuRoot,
  SubmenuTrigger: DropdownSubmenuTrigger,
} as const;

export const ContextMenu = {
  Root: BaseContextMenu.Root,
  Trigger: BaseContextMenu.Trigger,
  Content: ContextContent,
  Item: ContextItem,
  IconItem: ContextIconItem,
  Separator: ContextSeparator,
  SubmenuRoot: BaseContextMenu.SubmenuRoot,
  SubmenuTrigger: ContextSubmenuTrigger,
} as const;
