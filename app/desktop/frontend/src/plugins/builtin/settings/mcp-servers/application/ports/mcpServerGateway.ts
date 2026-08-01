import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { MCPServerInput } from "../mcpServerInput";

export interface MCPServerTestOutcome {
  ok: boolean;
  error?: string;
}

export interface MCPServerGateway {
  create(input: MCPServerInput): Promise<void>;
  update(name: string, input: MCPServerInput): Promise<void>;
  delete(name: string): Promise<void>;
  setEnabled(name: string, enabled: boolean): Promise<void>;
  authorize(name: string): Promise<void>;
  test(input: MCPServerInput): Promise<MCPServerTestOutcome>;
}

const port = createSingletonPort<MCPServerGateway>("MCP server gateway is not configured");

export const configureMCPServerGateway = port.configure;
export const mcpServerGateway = port.get;
