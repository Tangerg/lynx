import type { ComponentProps, ReactNode } from "react";
import { Popover as BasePopover } from "@base-ui/react/popover";
import { cn } from "@/lib/utils";

type PositionerProps = ComponentProps<typeof BasePopover.Positioner>;
type PopupProps = ComponentProps<typeof BasePopover.Popup>;

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
    <BasePopover.Portal>
      <BasePopover.Positioner
        side={side}
        align={align}
        sideOffset={sideOffset}
        alignOffset={alignOffset}
        className={positionerClassName}
      >
        <BasePopover.Popup
          {...popupProps}
          className={cn(
            "z-50 overflow-hidden rounded-md border-0 bg-surface shadow-[var(--shadow-popover)] animate-rise-in",
            className,
          )}
        >
          {children}
        </BasePopover.Popup>
      </BasePopover.Positioner>
    </BasePopover.Portal>
  );
}

export const Popover = {
  Root: BasePopover.Root,
  Trigger: BasePopover.Trigger,
  Content: PopoverContent,
} as const;
