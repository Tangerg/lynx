import type { TermLine } from "@/components/tools/previews";
import { cn } from "@/lib/utils";

// Terminal output viewer. Inline span runs colored by kind:
//   prompt  → accent green (the "$" / "❯")
//   cmd     → default fg (what the user typed)
//   ok/err/warn/mute → semantic / faint

const kindClass = (kind: TermLine["kind"]) =>
  kind === "prompt"
    ? "text-accent font-semibold"
    : kind === "cmd"
      ? "text-fg"
      : kind === "ok"
        ? "text-accent"
        : kind === "err"
          ? "text-negative"
          : kind === "warn"
            ? "text-warning"
            : /* mute */ "text-fg-faint";

export function Terminal({ lines, running }: { lines: TermLine[]; running: boolean }) {
  return (
    <div className="whitespace-pre px-4 py-3.5 font-mono text-[12px] leading-[1.55] text-fg-soft">
      {lines.map((l, i) => (
        <span key={i} className={cn(kindClass(l.kind))}>
          {l.text}
        </span>
      ))}
      {running && (
        <span className="mt-2 inline-flex items-center gap-2 text-info">
          <span className="h-2 w-2 rounded-full bg-current animate-pulse-dot" />
          tsc watching for changes…
        </span>
      )}
    </div>
  );
}
