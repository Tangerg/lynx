import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { PopoverPrimitive } from "@/ui/primitives";

type PositionerProps = ComponentProps<typeof PopoverPrimitive.Positioner>;
type PopupProps = ComponentProps<typeof PopoverPrimitive.Popup>;

interface PopoverContentBaseProps {
  children: ReactNode;
  className?: string;
  positionerClassName?: string;
  side?: PositionerProps["side"];
  align?: PositionerProps["align"];
  sideOffset?: PositionerProps["sideOffset"];
  alignOffset?: PositionerProps["alignOffset"];
}

type PopoverContentProps = PopoverContentBaseProps &
  Omit<PopupProps, keyof PopoverContentBaseProps | "className">;

function PopoverContent({
  children,
  className,
  positionerClassName,
  side,
  align,
  sideOffset,
  alignOffset,
  ...popupProps
}: PopoverContentProps) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        className={positionerClassName}
      >
        <PopoverPrimitive.Popup
          {...popupProps}
          className={cn(
            // Same frosted shell as the menus — see MENU_CONTENT_CLASSES.
            "relative z-50 overflow-hidden rounded-xl",
            "bg-canvas/70 shadow-[var(--shadow-popover)] animate-rise-in",
            "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
            "before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150",
            className,
          )}
        >
          {children}
        </PopoverPrimitive.Popup>
      </PopoverPrimitive.Positioner>
    </PopoverPrimitive.Portal>
  );
}

export const Popover = {
  Root: PopoverPrimitive.Root,
  Trigger: PopoverPrimitive.Trigger,
  Content: PopoverContent,
} as const;
