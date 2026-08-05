import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { PopoverPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";

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
        className={cn(FLOATING_LAYER, positionerClassName)}
      >
        <PopoverPrimitive.Popup {...popupProps} className={cn(FLOATING_PANEL, className)}>
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
