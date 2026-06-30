import type { ReactElement, ReactNode } from "react";
import { Tooltip as BaseTooltip } from "@base-ui/react/tooltip";
import { cn } from "@/lib/utils";

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
    <BaseTooltip.Provider delay={250} closeDelay={0} timeout={150}>
      {children}
    </BaseTooltip.Provider>
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
      className="max-w-[280px] px-2 py-1 font-sans text-[11.5px] leading-snug text-fg-soft"
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
    <BaseTooltip.Root>
      <BaseTooltip.Trigger render={trigger} delay={delay} />
      <BaseTooltip.Portal>
        <BaseTooltip.Positioner side={side} sideOffset={sideOffset}>
          <BaseTooltip.Popup
            className={cn(
              "z-50 rounded-md border-0 bg-surface shadow-[var(--shadow-popover)] animate-rise-in",
              className,
            )}
          >
            {children}
          </BaseTooltip.Popup>
        </BaseTooltip.Positioner>
      </BaseTooltip.Portal>
    </BaseTooltip.Root>
  );
}
