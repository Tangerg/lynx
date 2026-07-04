import { cn } from "@/lib/utils";
import { ProgressPrimitive } from "@/ui/primitives";

interface ProgressBarProps {
  value: number;
  className?: string;
  indicatorClassName?: string;
}

export function ProgressBar({ value, className, indicatorClassName }: ProgressBarProps) {
  const bounded = Math.max(0, Math.min(100, value));
  return (
    <ProgressPrimitive.Root
      value={bounded}
      className={cn("h-1 overflow-hidden rounded-full bg-surface-3", className)}
    >
      <ProgressPrimitive.Indicator
        className={cn("h-full bg-accent transition-[width] duration-150", indicatorClassName)}
        style={{ width: `${bounded}%` }}
      />
    </ProgressPrimitive.Root>
  );
}
