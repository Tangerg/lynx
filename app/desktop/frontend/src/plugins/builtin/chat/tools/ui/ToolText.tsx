import type { ToolDetail } from "@/plugins/builtin/agent/public/messagePresentation";
import { FilePath } from "@/ui";
import { cn } from "@/lib/classNames";

/**
 * One of the row's two written slots.
 *
 * A path keeps its filename when the row runs out of width, which is the whole
 * reason the model says which kind of value it is holding — and why both slots go
 * through here instead of one of them spelling it out.
 */
export function ToolText({ value, className }: { value: ToolDetail; className?: string }) {
  if (value.kind === "path") {
    return <FilePath path={value.value} className={className} />;
  }
  return (
    <span className={cn("truncate", className)} title={value.value}>
      {value.value}
    </span>
  );
}
