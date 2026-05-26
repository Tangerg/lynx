// Generic tool inspector — fallback when a tool fn has no
// plugin-registered preview. Surfaces the raw `args` + `result` from
// the AG-UI event so the user can see exactly what the agent passed
// in and got back, even for tools we've never seen before.
//
// Portai's ToolInvocationCard does the same: it pretty-prints JSON,
// keeps non-JSON text as-is, and clearly labels the two halves. The
// inline preview is for "I want a quick visual"; this is for "I want
// the truth".

import type { ToolCall } from "@/protocol/agui/viewState";
import { useMemo } from "react";
import { cn } from "@/lib/utils";

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
  const args = useMemo(() => formatBody(tool.args), [tool.args]);
  const result = useMemo(() => formatBody(tool.result), [tool.result]);

  return (
    <div className="bg-canvas px-3.5 py-2.5">
      <InspectorSection title="Arguments" body={args} />
      {result.text && <InspectorSection title="Result" body={result} />}
      {!result.text && tool.status === "ok" && (
        <div className="font-mono text-[11px] text-fg-faint">
          (no result body — tool returned empty)
        </div>
      )}
    </div>
  );
}

function InspectorSection({ title, body }: { title: string; body: FormattedBody }) {
  if (!body.text) return null;
  return (
    <div className="mb-2 last:mb-0">
      <div className="mb-1 flex items-baseline gap-2">
        <span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-fg-faint">
          {title}
        </span>
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
