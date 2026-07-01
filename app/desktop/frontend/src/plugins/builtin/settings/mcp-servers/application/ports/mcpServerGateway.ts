import type { MCPServerConfigInput } from "../mcpServerInput";

export interface MCPServerTestOutcome {
  ok: boolean;
  error?: string;
}

export interface MCPServerGateway {
  configure(input: MCPServerConfigInput): Promise<void>;
  remove(name: string): Promise<void>;
  setEnabled(name: string, enabled: boolean): Promise<void>;
  authorize(name: string): Promise<void>;
  test(input: MCPServerConfigInput): Promise<MCPServerTestOutcome>;
}

let port: MCPServerGateway | null = null;

export function configureMCPServerGateway(next: MCPServerGateway): void {
  port = next;
}

export function mcpServerGateway(): MCPServerGateway {
  if (!port) throw new Error("MCP server gateway is not configured");
  return port;
}
