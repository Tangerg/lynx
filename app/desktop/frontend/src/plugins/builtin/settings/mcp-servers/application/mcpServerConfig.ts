import { useCallback } from "react";
import { queryClient } from "@/lib/queryClient";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type MCPServerSettings,
  useMCPServers,
} from "./mcpServerQueries";
import type { MCPServerInput } from "./mcpServerInput";
import { mcpServerGateway, type MCPServerTestOutcome } from "./ports/mcpServerGateway";

const AUTHORIZATION_ATTEMPT_POLL_MS = 500;

export type { MCPServerInput } from "./mcpServerInput";
export type { MCPTransport } from "./mcpServerQueries";
export type { MCPServerTestOutcome } from "./ports/mcpServerGateway";
export type { MCPServerSettings };

export function useMCPServerConfigs() {
  return useMCPServers();
}

function invalidateMcp(): Promise<void> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: [MCP_SERVERS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [MCP_TOOLS_KEY] }),
  ]).then(() => undefined);
}

export function useCreateMCPServer(): (input: MCPServerInput) => Promise<void> {
  return useCallback(async (input) => {
    await mcpServerGateway().create(input);
    await invalidateMcp();
  }, []);
}

export function useUpdateMCPServer(): (name: string, input: MCPServerInput) => Promise<void> {
  return useCallback(async (name, input) => {
    await mcpServerGateway().update(name, input);
    await invalidateMcp();
  }, []);
}

export function useDeleteMCPServer(): (name: string) => Promise<void> {
  return useCallback(async (name) => {
    await mcpServerGateway().delete(name);
    await invalidateMcp();
  }, []);
}

export function useSetMCPServerEnabled(): (name: string, enabled: boolean) => Promise<void> {
  return useCallback(async (name, enabled) => {
    await mcpServerGateway().setEnabled(name, enabled);
    await invalidateMcp();
  }, []);
}

export function useAuthorizeMCPServer(): (name: string, signal?: AbortSignal) => Promise<void> {
  return useCallback(authorizeMCPServer, []);
}

export async function authorizeMCPServer(name: string, signal?: AbortSignal): Promise<void> {
  const gateway = mcpServerGateway();
  let attempt = await gateway.createAuthorizationAttempt(name, signal);
  while (attempt.status === "pending") {
    await authorizationPollDelay(signal);
    attempt = await gateway.getAuthorizationAttempt(attempt.id, signal);
  }
  await invalidateMcp();
  if (attempt.status === "failed") {
    throw new Error(attempt.error);
  }
}

function authorizationPollDelay(signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) return Promise.reject(signal.reason);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(done, AUTHORIZATION_ATTEMPT_POLL_MS);
    function done(): void {
      clearTimeout(timer);
      signal?.removeEventListener("abort", aborted);
      resolve();
    }
    function aborted(): void {
      clearTimeout(timer);
      reject(signal?.reason);
    }
    signal?.addEventListener("abort", aborted, { once: true });
  });
}

export function useTestMCPServer(): (input: MCPServerInput) => Promise<MCPServerTestOutcome> {
  return useCallback((input) => mcpServerGateway().test(input), []);
}
