import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolCategory } from "@/plugins/builtin/agent/public/viewState";

export type WorkspaceToolActivityCategory = "command" | "fileEdit" | "read" | "inline";

export interface WorkspaceToolActivity {
  id: string;
  category: WorkspaceToolActivityCategory;
  label: string;
}

export interface WorkspaceCommandActivity {
  id: string;
  command: string;
  status: "running" | "succeeded" | "failed" | "blocked";
  output: string;
  outputTruncated: boolean;
  exitCode?: number;
}

export function workspaceToolActivityFromAgentTool(tool: ToolCall): WorkspaceToolActivity {
  const category = toolCategory(tool.name);
  return {
    id: tool.id,
    category:
      category === "command" || category === "fileEdit" || category === "read"
        ? category
        : "inline",
    label: tool.fn,
  };
}

function workspaceCommandActivityFromAgentTool(tool: ToolCall): WorkspaceCommandActivity {
  return {
    id: tool.id,
    command: tool.fn,
    status: workspaceCommandStatus(tool.status),
    output: tool.result ?? "",
    outputTruncated: tool.outputTruncated ?? false,
    exitCode: tool.exitCode,
  };
}

function workspaceCommandStatus(status: ToolCall["status"]): WorkspaceCommandActivity["status"] {
  switch (status) {
    case "running":
      return "running";
    case "ok":
      return "succeeded";
    case "err":
      return "failed";
    case "denied":
    case "requires-action":
      return "blocked";
  }
}

export function workspaceCommandActivitiesFromAgentTools(
  toolCalls: Record<string, ToolCall>,
): WorkspaceCommandActivity[] {
  return Object.values(toolCalls)
    .filter((tool) => workspaceToolActivityFromAgentTool(tool).category === "command")
    .map(workspaceCommandActivityFromAgentTool);
}
