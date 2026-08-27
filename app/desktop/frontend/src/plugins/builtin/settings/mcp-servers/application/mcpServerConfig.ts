import { useCallback, useSyncExternalStore } from "react";
import { type MCPServerSettings, useMCPServers } from "./mcpServerQueries";
import type { MCPServerInput } from "./mcpServerInput";
import type { MCPServerTestOutcome } from "./ports/mcpServerGateway";
import { MCPServerMutationOwner, mcpServerMutationWasRetired } from "./mcpServerMutationOwner";

export type { MCPServerInput } from "./mcpServerInput";
export type { MCPTransport } from "./mcpServerQueries";
export type { MCPServerTestOutcome } from "./ports/mcpServerGateway";
export type { MCPServerSettings };
export { mcpServerMutationWasRetired };

export function useMCPServerConfigs() {
  return useMCPServers();
}

export function useMCPServerMutationMaterialGeneration(): number {
  return useSyncExternalStore(
    MCPServerMutationOwner.subscribeMaterialGeneration,
    MCPServerMutationOwner.materialGeneration,
    MCPServerMutationOwner.materialGeneration,
  );
}

export function createMCPServer(input: MCPServerInput): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().create(input);
}

export function updateMCPServer(name: string, input: MCPServerInput): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().update(name, input);
}

export function setMCPServerEnabled(name: string, enabled: boolean): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().setEnabled(name, enabled);
}

export function deleteMCPServer(name: string): Promise<void> {
  return MCPServerMutationOwner.current().delete(name);
}

export function reconnectMCPServer(name: string): Promise<void> {
  return MCPServerMutationOwner.current().reconnect(name);
}

export function useCreateMCPServer(): (input: MCPServerInput) => Promise<void> {
  return useCallback((input) => createMCPServer(input).then(() => undefined), []);
}

export function useUpdateMCPServer(): (name: string, input: MCPServerInput) => Promise<void> {
  return useCallback((name, input) => updateMCPServer(name, input).then(() => undefined), []);
}

export function useDeleteMCPServer(): (name: string) => Promise<void> {
  return useCallback((name) => deleteMCPServer(name), []);
}

export function useSetMCPServerEnabled(): (name: string, enabled: boolean) => Promise<void> {
  return useCallback(
    (name, enabled) => setMCPServerEnabled(name, enabled).then(() => undefined),
    [],
  );
}

export function useAuthorizeMCPServer(): (name: string, signal?: AbortSignal) => Promise<void> {
  return useCallback((name, signal) => authorizeMCPServer(name, signal), []);
}

export async function authorizeMCPServer(name: string, signal?: AbortSignal): Promise<void> {
  await MCPServerMutationOwner.current().authorize(name, signal);
}

export function useTestMCPServer(): (input: MCPServerInput) => Promise<MCPServerTestOutcome> {
  return useCallback((input) => MCPServerMutationOwner.current().test(input), []);
}
