import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenuPrimitive, MenuPrimitive } from "@/ui/primitives";

const MENU_CONTENT_CLASSES =
  "z-50 overflow-hidden rounded-[12px] bg-canvas p-1 shadow-[var(--shadow-popover)] animate-rise-in";

const MENU_ITEM_CLASSES =
  "grid h-8 items-center gap-2 rounded-md px-2.5 text-[13px] text-fg outline-none data-[highlighted]:bg-fg/[0.06]";

const MENU_SEPARATOR_CLASSES = "mx-1 my-1 h-px bg-fg/[0.06]";

type DropdownPositionerProps = ComponentProps<typeof MenuPrimitive.Positioner>;
type DropdownPopupProps = ComponentProps<typeof MenuPrimitive.Popup>;
type ContextPopupProps = ComponentProps<typeof ContextMenuPrimitive.Popup>;

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

type DropdownItemProps = ComponentProps<typeof MenuPrimitive.Item>;
type DropdownSubmenuTriggerProps = ComponentProps<typeof MenuPrimitive.SubmenuTrigger>;
type ContextItemProps = ComponentProps<typeof ContextMenuPrimitive.Item>;
type ContextSubmenuTriggerProps = ComponentProps<typeof ContextMenuPrimitive.SubmenuTrigger>;

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
    <MenuPrimitive.Portal>
      <MenuPrimitive.Positioner
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
        <MenuPrimitive.Popup {...popupProps} className={cn(MENU_CONTENT_CLASSES, className)}>
          {children}
        </MenuPrimitive.Popup>
      </MenuPrimitive.Positioner>
    </MenuPrimitive.Portal>
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
    <ContextMenuPrimitive.Portal>
      <ContextMenuPrimitive.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        className={cn("z-50", positionerClassName)}
      >
        <ContextMenuPrimitive.Popup {...popupProps} className={cn(MENU_CONTENT_CLASSES, className)}>
          {children}
        </ContextMenuPrimitive.Popup>
      </ContextMenuPrimitive.Positioner>
    </ContextMenuPrimitive.Portal>
  );
}

function DropdownSeparator({
  className,
  ...props
}: ComponentProps<typeof MenuPrimitive.Separator>) {
  return <MenuPrimitive.Separator {...props} className={cn(MENU_SEPARATOR_CLASSES, className)} />;
}

function ContextSeparator({
  className,
  ...props
}: ComponentProps<typeof ContextMenuPrimitive.Separator>) {
  return (
    <ContextMenuPrimitive.Separator {...props} className={cn(MENU_SEPARATOR_CLASSES, className)} />
  );
}

function DropdownItem({ className, ...props }: DropdownItemProps) {
  return <MenuPrimitive.Item {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function DropdownSubmenuTrigger({ className, ...props }: DropdownSubmenuTriggerProps) {
  return <MenuPrimitive.SubmenuTrigger {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function ContextItem({ className, ...props }: ContextItemProps) {
  return <ContextMenuPrimitive.Item {...props} className={cn(MENU_ITEM_CLASSES, className)} />;
}

function ContextSubmenuTrigger({ className, ...props }: ContextSubmenuTriggerProps) {
  return (
    <ContextMenuPrimitive.SubmenuTrigger {...props} className={cn(MENU_ITEM_CLASSES, className)} />
  );
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
  Root: MenuPrimitive.Root,
  Trigger: MenuPrimitive.Trigger,
  Content: DropdownContent,
  Item: DropdownItem,
  Separator: DropdownSeparator,
  SubmenuRoot: MenuPrimitive.SubmenuRoot,
  SubmenuTrigger: DropdownSubmenuTrigger,
} as const;

export const ContextMenu = {
  Root: ContextMenuPrimitive.Root,
  Trigger: ContextMenuPrimitive.Trigger,
  Content: ContextContent,
  Item: ContextItem,
  IconItem: ContextIconItem,
  Separator: ContextSeparator,
  SubmenuRoot: ContextMenuPrimitive.SubmenuRoot,
  SubmenuTrigger: ContextSubmenuTrigger,
} as const;
