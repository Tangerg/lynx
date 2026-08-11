import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";

export type WorkspaceCapability = "git" | "knowledge" | "skills" | "plan";

export function useWorkspaceCapability(capability: WorkspaceCapability): boolean {
  return useRuntimeCapability(capability);
}
