import type { MCPServerInput } from "../mcpServerInput";
import type { MCPServerSettings } from "../mcpServerQueries";

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
  create(input: MCPServerInput): Promise<MCPServerSettings>;
  update(name: string, input: MCPServerInput): Promise<MCPServerSettings>;
  delete(name: string): Promise<void>;
  setEnabled(name: string, enabled: boolean): Promise<MCPServerSettings>;
  reconnect(name: string): Promise<void>;
  createAuthorizationAttempt(name: string, signal?: AbortSignal): Promise<MCPAuthorizationAttempt>;
  getAuthorizationAttempt(id: string, signal?: AbortSignal): Promise<MCPAuthorizationAttempt>;
  test(input: MCPServerInput): Promise<MCPServerTestOutcome>;
}
