import type { ReactElement, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { TooltipPrimitive } from "@/ui/primitives";

export interface TooltipProviderProps {
  children: ReactNode;
}

interface Props {
  label?: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delayDuration?: number;
  children: ReactNode;
}

interface RichTooltipProps {
  trigger: ReactElement;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  delay?: number;
  className?: string;
}

export function TooltipProvider({ children }: TooltipProviderProps) {
  return (
    <TooltipPrimitive.Provider delay={250} closeDelay={0} timeout={150}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export function Tooltip({ label, side = "top", sideOffset = 6, delayDuration, children }: Props) {
  if (label == null || label === "") return <>{children}</>;
  return (
    <RichTooltip
      trigger={children as ReactElement}
      side={side}
      sideOffset={sideOffset}
      delay={delayDuration}
      className="max-w-[280px] bg-fg px-2 py-1 font-sans text-ui-md leading-snug text-on-fg"
    >
      {label}
    </RichTooltip>
  );
}

export function RichTooltip({
  trigger,
  children,
  side = "top",
  sideOffset = 6,
  delay,
  className,
}: RichTooltipProps) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger render={trigger} delay={delay} />
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Positioner side={side} sideOffset={sideOffset}>
          <TooltipPrimitive.Popup
            className={cn(
              "relative z-50 overflow-hidden rounded-md",
              "bg-canvas/70 text-fg shadow-[var(--shadow-popover)] animate-rise-in",
              "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
              "before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150",
              className,
            )}
          >
            {children}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
