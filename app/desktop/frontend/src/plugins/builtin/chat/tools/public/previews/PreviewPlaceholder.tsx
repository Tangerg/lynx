import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useT } from "@/lib/i18n";

// Muted empty-state line for a tool preview: the `pending` copy while the call
// is still running, the `idle` copy once it settled with nothing to show. Both are
// i18n keys, resolved here — same contract as PreviewFoot's `label`.
// Unifies the repeated `tool.status === "running" ? … : …` placeholders across
// the built-in previews so they read (and tone) the same.
export function PreviewPlaceholder({
  status,
  pending,
  idle,
}: {
  status: ToolCall["status"];
  pending: string;
  idle: string;
}) {
  const t = useT();
  return <div className="text-fg-faint">{t(status === "running" ? pending : idle)}</div>;
}
