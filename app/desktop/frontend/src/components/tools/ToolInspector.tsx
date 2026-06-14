// Generic tool inspector — fallback when a tool fn has no
// plugin-registered preview. Surfaces the raw `args` + `result` from
// the StreamEvent so the user can see exactly what the agent passed
// in and got back, even for tools we've never seen before.
//
// It pretty-prints JSON, keeps non-JSON text as-is, and clearly labels
// the two halves: the inline preview is for "I want a quick visual",
// this is for "I want the truth".

import type { ToolCall } from "@/protocol/run/viewState";
import { useMemo } from "react";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

interface FormattedBody {
  text: string;
  isJson: boolean;
}

function formatBody(raw: string | undefined): FormattedBody {
  if (!raw) return { text: "", isJson: false };
  const trimmed = raw.trim();
  if (!trimmed) return { text: "", isJson: false };
  // Only attempt JSON parse if the text looks like a structure; saves
  // us from parsing a 5KB bash stdout that starts with a `{` and fails
  // halfway.
  if (trimmed[0] === "{" || trimmed[0] === "[") {
    try {
      return { text: JSON.stringify(JSON.parse(trimmed), null, 2), isJson: true };
    } catch {
      /* fall through to raw */
    }
  }
  return { text: raw, isJson: false };
}

export function ToolInspector({ tool }: { tool: ToolCall }) {
  const t = useT();
  const args = useMemo(() => formatBody(tool.args), [tool.args]);
  const result = useMemo(() => formatBody(tool.result), [tool.result]);

  return (
    <div className="bg-canvas px-3.5 py-2.5">
      <InspectorSection title={t("toolInspector.arguments")} body={args} />
      {result.text && <InspectorSection title={t("toolInspector.result")} body={result} />}
      {!result.text && tool.status === "ok" && (
        <div className="font-mono text-[11px] text-fg-faint">{t("toolInspector.noResult")}</div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: FormattedBody }) {
  if (!body.text) return null;
  return (
    <div className="mb-2 last:mb-0">
      <div className="mb-1 flex items-baseline gap-2">
        <span className="font-mono text-[10px] font-semibold text-fg-faint">{title}</span>
        {body.isJson && <span className="font-mono text-[10px] text-fg-faint">json</span>}
      </div>
      <pre
        className={cn(
          "max-h-60 overflow-y-auto rounded-sm bg-surface-2 px-2.5 py-2 font-mono text-[11.5px] leading-[1.55] text-fg-soft",
          // JSON shows whitespace-pre to preserve indentation; raw text
          // wraps so long stdout / stderr lines stay readable.
          body.isJson ? "whitespace-pre" : "whitespace-pre-wrap break-all",
        )}
      >
        {body.text}
      </pre>
    </div>
  );
}
