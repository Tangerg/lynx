import type {VariantProps} from "class-variance-authority";
import { cva  } from "class-variance-authority";
import { cn } from "@/lib/utils";

// Small colored dot used as a status indicator (running / waiting / idle).
// Used in tab strips, session rows, run badges. The accent variant pulses
// to draw the eye when the agent is live.
const dotStyles = cva("inline-block h-1.5 w-1.5 shrink-0 rounded-full", {
  variants: {
    tone: {
      idle: "bg-fg-faint",
      running: "bg-accent shadow-[0_0_6px_var(--color-accent)] animate-pulse-dot",
      waiting: "bg-warning",
      ok: "bg-success",
      err: "bg-negative",
    },
  },
  defaultVariants: { tone: "idle" },
});

type Props = VariantProps<typeof dotStyles> & { className?: string };

export function StatusDot({ tone, className }: Props) {
  return <span className={cn(dotStyles({ tone }), className)} />;
}
