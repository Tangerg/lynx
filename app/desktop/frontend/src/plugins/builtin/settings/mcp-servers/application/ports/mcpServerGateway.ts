import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<MCPServerGateway>("MCP server gateway is not configured");

export const configureMCPServerGateway = port.configure;
export const mcpServerGateway = port.get;
