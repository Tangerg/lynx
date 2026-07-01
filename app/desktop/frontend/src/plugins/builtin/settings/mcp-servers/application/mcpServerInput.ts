export type MCPServerTransport = "stdio" | "streamableHttp";

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
