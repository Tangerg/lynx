import type { ToolCall } from "@/plugins/sdk/types/agentView";
import { toolCategory } from "@/plugins/sdk/types/agentView";

export type WorkspaceToolActivityCategory = "command" | "fileEdit" | "read" | "inline";

export interface WorkspaceToolActivity {
  id: string;
  category: WorkspaceToolActivityCategory;
  label: string;
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
