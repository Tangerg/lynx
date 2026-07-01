import type { MemoryEntryInfo } from "@/lib/data/queries";
import { MEMORY_KEY, useMemory } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";

export type WorkspaceMemoryEntry = MemoryEntryInfo;

export function useWorkspaceMemory(enabled: boolean, cwd?: string) {
  return useMemory(enabled ? { cwd } : undefined);
}

export async function saveWorkspaceMemory(input: {
  scope: WorkspaceMemoryEntry["scope"];
  cwd?: string;
  content: string;
}): Promise<void> {
  await getContainer().client().memory.update(input);
  await queryClient.invalidateQueries({ queryKey: [MEMORY_KEY] });
}
