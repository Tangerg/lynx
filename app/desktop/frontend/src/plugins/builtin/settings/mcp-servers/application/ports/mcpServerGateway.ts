import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { MCPServerInput } from "../mcpServerInput";

export interface MCPServerTestOutcome {
  ok: boolean;
  error?: string;
}

export type MCPAuthorizationAttempt =
  | { id: string; status: "pending" }
  | { id: string; status: "succeeded" }
  | { id: string; status: "failed"; error: string }
  | { id: string; status: "canceled" };

export interface MCPServerGateway {
  create(input: MCPServerInput): Promise<void>;
  update(name: string, input: MCPServerInput): Promise<void>;
  delete(name: string): Promise<void>;
  setEnabled(name: string, enabled: boolean): Promise<void>;
  createAuthorizationAttempt(name: string, signal?: AbortSignal): Promise<MCPAuthorizationAttempt>;
  getAuthorizationAttempt(id: string, signal?: AbortSignal): Promise<MCPAuthorizationAttempt>;
  test(input: MCPServerInput): Promise<MCPServerTestOutcome>;
}

const port = createSingletonPort<MCPServerGateway>("MCP server gateway is not configured");

export const configureMCPServerGateway = port.configure;
export const mcpServerGateway = port.get;
