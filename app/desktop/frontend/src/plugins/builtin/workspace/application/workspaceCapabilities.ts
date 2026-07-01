import { useServerFeature } from "@/state/runtimeStore";

export type WorkspaceCapability = "git" | "memory" | "skills" | "todos";

export function useWorkspaceCapability(capability: WorkspaceCapability): boolean {
  return useServerFeature(capability);
}
