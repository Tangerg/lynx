import { Progress as BaseProgress } from "@base-ui/react/progress";
import { cn } from "@/lib/utils";

interface ProgressBarProps {
  value: number;
  className?: string;
  indicatorClassName?: string;
}

export function ProgressBar({ value, className, indicatorClassName }: ProgressBarProps) {
  const bounded = Math.max(0, Math.min(100, value));
  return (
    <BaseProgress.Root
      value={bounded}
      className={cn("h-1 overflow-hidden rounded-full bg-surface-3", className)}
    >
      <BaseProgress.Indicator
        className={cn("h-full bg-accent transition-[width] duration-150", indicatorClassName)}
        style={{ width: `${bounded}%` }}
      />
    </BaseProgress.Root>
  );
}
