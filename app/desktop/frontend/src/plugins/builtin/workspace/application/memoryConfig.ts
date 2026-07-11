import { queryClient } from "@/lib/data/queryClient";
import { workspaceMemoryGateway, type WorkspaceMemoryUpdateInput } from "./ports/memoryGateway";
import {
  WORKSPACE_MEMORY_KEY,
  useWorkspaceMemory as useMemoryQuery,
  type WorkspaceMemoryEntry as MemoryEntryInfo,
} from "./workspaceData";

export type WorkspaceMemoryEntry = MemoryEntryInfo;

export function useWorkspaceMemory(enabled: boolean, cwd?: string) {
  return useMemoryQuery(enabled ? { cwd } : undefined);
}

export async function saveWorkspaceMemory(input: WorkspaceMemoryUpdateInput): Promise<void> {
  await workspaceMemoryGateway().save(input);
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_MEMORY_KEY] });
}
