import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";

export type WorkspaceCapability = "git" | "memory" | "skills" | "todos";

export function useWorkspaceCapability(capability: WorkspaceCapability): boolean {
  return useRuntimeCapability(capability);
}
