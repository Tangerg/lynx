import type { ReactElement, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { TooltipPrimitive } from "@/ui/primitives";
import { FLOATING_LAYER, FLOATING_TIP } from "./floating-surface";

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
        {/* The z-index belongs on the POSITIONER, not on the popup. The
            positioner carries a transform, which makes it a stacking context, so
            the popup's own `z-50` only ranks it against its siblings inside that
            box — and the box itself, at `z-index: auto`, then stacks by DOM order
            and loses to anything on the page that owns a context. Every tooltip
            in the app was rendering *behind* the content it described. Menu makes
            the same move for the same reason. */}
        <TooltipPrimitive.Positioner className={FLOATING_LAYER} side={side} sideOffset={sideOffset}>
          <TooltipPrimitive.Popup role="tooltip" className={cn(FLOATING_TIP, className)}>
            {children}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
