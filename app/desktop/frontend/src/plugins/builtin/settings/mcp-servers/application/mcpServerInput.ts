import type { MCPTransport } from "./mcpServerQueries";

export interface MCPServerInput {
  name: string;
  transport: MCPTransport;
  enabled: boolean;
  description?: string;
  command?: string;
  args?: string[];
  // Secret maps use undefined=preserve, null=clear, and a map=replace.
  env?: Record<string, string> | null;
  dir?: string;
  url?: string;
  // undefined preserves an existing credential, null clears it, and a string
  // sets it. New resources use undefined for no credential.
  authorization?: string | null;
  headers?: Record<string, string> | null;
  timeoutSeconds?: number;
  disabledTools?: string[];
  autoApproveTools?: string[];
}
