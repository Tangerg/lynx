import type { MemoryEntryInfo } from "@/lib/data/queries";
import { MEMORY_KEY, useMemory } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { workspaceMemoryGateway, type WorkspaceMemoryUpdateInput } from "./ports/memoryGateway";

export type WorkspaceMemoryEntry = MemoryEntryInfo;

export function useWorkspaceMemory(enabled: boolean, cwd?: string) {
  return useMemory(enabled ? { cwd } : undefined);
}

export async function saveWorkspaceMemory(input: WorkspaceMemoryUpdateInput): Promise<void> {
  await workspaceMemoryGateway().save(input);
  await queryClient.invalidateQueries({ queryKey: [MEMORY_KEY] });
}
