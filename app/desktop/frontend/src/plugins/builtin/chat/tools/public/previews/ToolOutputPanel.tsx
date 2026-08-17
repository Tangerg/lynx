import { useMemo, useState } from "react";
import { type AnsiSpan, type AnsiTone, hasAnsi, parseAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import { Badge, Icon, IconButton, TextButton } from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";

/** How much output a row shows before it offers the rest. Nine lines is about what
 *  reads as "a glance at the output" rather than "the output". */
const COLLAPSED_LINES = 9;

const TONE_CLASS: Record<AnsiTone, string> = {
  negative: "text-negative",
  success: "text-success",
  warning: "text-warning",
  info: "text-info",
  accent: "text-accent",
  muted: "text-fg-faint",
};

function spanClass(span: AnsiSpan): string | undefined {
  const parts = [
    span.tone ? TONE_CLASS[span.tone] : undefined,
    span.bold ? "font-semibold" : undefined,
    span.dim ? "opacity-70" : undefined,
    span.underline ? "underline" : undefined,
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" ") : undefined;
}

/**
 * One line of program output.
 *
 * Plain text goes through `LinkedText`, which turns `path:line` into something
 * clickable — the single most useful thing in a compiler's output. A coloured line
 * cannot: the tones split it into spans, and a reference broken across two of them is
 * not a reference. Colour wins there, because a line the tool went out of its way to
 * paint red is a line the reader is looking for.
 */
function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return (
    <>
      {parseAnsi(text).map((span, index) => (
        <span key={index} className={spanClass(span)}>
          {span.text}
        </span>
      ))}
    </>
  );
}

interface ToolOutputPanelProps {
  /** The merged output as the runtime projected it. */
  output: string | undefined;
  status: ToolCall["status"];
  /** The runtime capped the output — the reader has to know the tail is missing. */
  truncated?: boolean;
  /** What the placeholder says while there is nothing yet. */
  idleLabel?: string;
}

/**
 * The panel that holds program output in a tool row.
 *
 * One panel, for every tool whose result is text a program wrote, because the parts
 * that make output readable were missing from all of them and worth writing once: the
 * escapes it is coloured with, a count of what is being withheld, a way to get the
 * rest, and a way to take it somewhere else. A preview that shows nine lines and
 * nothing else makes the reader open the full view to learn whether it was worth
 * opening.
 */
export function ToolOutputPanel({
  output,
  status,
  truncated,
  idleLabel = "tools.preview.idle.noOutput",
}: ToolOutputPanelProps) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);

  const lines = useMemo(() => {
    const trimmed = output?.replace(/\n+$/, "") ?? "";
    return trimmed === "" ? [] : trimmed.split("\n");
  }, [output]);
  const copyMaterial = lines.join("\n");
  const { copied, copy } = useCopyFeedback(copyMaterial);

  const hidden = lines.length - COLLAPSED_LINES;
  const shown = expanded ? lines : lines.slice(0, COLLAPSED_LINES);

  if (lines.length === 0) {
    return (
      <div className="rounded-sm bg-sunken px-3 py-2.5 font-mono text-code leading-relaxed">
        <PreviewPlaceholder
          status={status}
          pending="tools.preview.pending.running"
          idle={idleLabel}
        />
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-sm bg-sunken">
      {/* No exit code here: the row's own meta column already carries it (see
          toolPresentation), and one fact said twice in one card is what the plan
          banner and the plan tool row were doing to each other. */}
      {truncated && (
        <div className="px-3 pt-2.5">
          <Badge>{t("tools.overflow.truncated")}</Badge>
        </div>
      )}
      <div className="group/output relative">
        {/* No ligatures. This is program output, not source: the mono face turns
            `===` and `!=` into single glyphs, which is right in a code block and wrong
            here — a test runner's `=== RUN` came out as a triple bar. */}
        <div className="overflow-x-auto px-3 py-2.5 font-mono text-code leading-relaxed text-fg-soft [font-variant-ligatures:none]">
          {shown.map((line, index) => (
            <div key={index} className="whitespace-pre-wrap wrap-anywhere">
              <OutputLine text={line} />
            </div>
          ))}
        </div>
        {/* Revealed on hover or focus, at the corner it does not cover text in. */}
        <IconButton
          icon={copied ? "check" : "copy"}
          size="xs"
          title={t(copied ? "tools.output.copied" : "tools.output.copy")}
          onClick={() => void copy()}
          className={cn(
            "absolute right-1 top-1 opacity-0 transition-opacity",
            "group-hover/output:opacity-100 group-focus-within/output:opacity-100",
          )}
        />
      </div>
      {hidden > 0 && (
        <div className="relative">
          {/* The fade is what says "there is more" before the label does — and it is
              drawn over the last line rather than after it, so the cut reads as the
              text continuing under it instead of stopping. */}
          {!expanded && (
            <div className="pointer-events-none absolute -top-6 inset-x-0 h-6 bg-[linear-gradient(to_top,var(--color-sunken),transparent)]" />
          )}
          <TextButton
            onClick={() => setExpanded((value) => !value)}
            className="w-full justify-center py-1.5 text-ui-sm hover:bg-hover"
          >
            <Icon name={expanded ? "chevron-up" : "chevron-down"} size="xs" />
            {expanded
              ? t("tools.output.collapse")
              : t("tools.output.showAll", { count: lines.length })}
          </TextButton>
        </div>
      )}
    </div>
  );
}
