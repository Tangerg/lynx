import { cn } from "@/lib/classNames";
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
      className={cn("h-1.5 overflow-hidden rounded-pill bg-sunken", className)}
    >
      <ProgressPrimitive.Indicator
        className={cn(
          "h-full rounded-pill bg-accent transition-[width] duration-[var(--dur-fast)]",
          indicatorClassName,
        )}
        style={{ width: `${bounded}%` }}
      />
    </ProgressPrimitive.Root>
  );
}
