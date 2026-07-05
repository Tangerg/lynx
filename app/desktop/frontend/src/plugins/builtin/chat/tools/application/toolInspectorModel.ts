import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";

export interface ToolInspectorBody {
  text: string;
  isJson: boolean;
}

export interface ToolInspectorModel {
  args: ToolInspectorBody;
  result: ToolInspectorBody;
  showNoResult: boolean;
}

export function formatToolInspectorBody(raw: string | undefined): ToolInspectorBody {
  if (!raw) return emptyBody();
  const trimmed = raw.trim();
  if (!trimmed) return emptyBody();
  if (looksStructured(trimmed)) {
    try {
      return { text: JSON.stringify(JSON.parse(trimmed), null, 2), isJson: true };
    } catch {
      return { text: raw, isJson: false };
    }
  }
  return { text: raw, isJson: false };
}

export function toolInspectorModel(
  tool: Pick<ToolCall, "args" | "result" | "status">,
): ToolInspectorModel {
  const args = formatToolInspectorBody(tool.args);
  const result = formatToolInspectorBody(tool.result);
  return {
    args,
    result,
    showNoResult: !result.text && tool.status === "ok",
  };
}

function emptyBody(): ToolInspectorBody {
  return { text: "", isJson: false };
}

function looksStructured(value: string): boolean {
  return value[0] === "{" || value[0] === "[";
}
