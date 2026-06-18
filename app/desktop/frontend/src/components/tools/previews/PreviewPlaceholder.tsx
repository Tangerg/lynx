import type { ToolCall } from "@/protocol/run/viewState";

// Muted empty-state line for a tool preview: the `pending` text while the call
// is still running, the `idle` text once it settled with nothing to show.
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
  return <div className="text-fg-faint">{status === "running" ? pending : idle}</div>;
}
