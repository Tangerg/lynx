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

export function useAuthorizeMCPServer(): (name: string) => Promise<void> {
  return useCallback(async (name) => {
    await mcpServerGateway().authorize(name);
    await invalidateMcp();
  }, []);
}

export function useTestMCPServer(): (input: MCPServerInput) => Promise<MCPServerTestOutcome> {
  return useCallback((input) => mcpServerGateway().test(input), []);
}
