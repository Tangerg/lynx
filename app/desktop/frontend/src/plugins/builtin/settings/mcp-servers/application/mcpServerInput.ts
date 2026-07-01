import type { MCPTransport } from "@/lib/data/queries";

export type MCPServerTransport = MCPTransport;

export interface MCPServerConfigInput {
  name: string;
  transport: MCPServerTransport;
  enabled: boolean;
  description?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  dir?: string;
  url?: string;
  authorization?: string;
  headers?: Record<string, string>;
  timeoutSeconds?: number;
  disabledTools?: string[];
  autoApproveTools?: string[];
}
