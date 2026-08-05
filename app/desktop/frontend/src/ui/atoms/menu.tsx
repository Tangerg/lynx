import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenuPrimitive, MenuPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";
import { floatingRowStyles } from "./option-row";

const MENU_CONTENT_CLASSES = `${FLOATING_PANEL} p-1`;

const MENU_ITEM_CLASSES = `relative ${floatingRowStyles({ size: "sm" })}`;

const MENU_SEPARATOR_CLASSES = "relative mx-1 my-1 h-px bg-divider";

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
        className={cn(FLOATING_LAYER, positionerClassName)}
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
        className={cn(FLOATING_LAYER, positionerClassName)}
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
          "text-negative data-[highlighted]:bg-negative-wash data-[highlighted]:text-negative",
        className,
      )}
    >
      <Icon name={icon} size="xs" />
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
