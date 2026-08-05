export const VISUAL_WORKSPACE_STATES = [
  "dock-light",
  "dock-review",
  "dock-inbox",
  "dock-stats",
  "dock-file",
  "dock-empty",
  "dock-loading",
  "dock-error",
  "settings",
] as const;

export type VisualWorkspaceState = (typeof VISUAL_WORKSPACE_STATES)[number];
export type VisualWorkspaceTheme = "light" | "dark";

export function isVisualWorkspaceState(value: string | null): value is VisualWorkspaceState {
  return VISUAL_WORKSPACE_STATES.includes(value as VisualWorkspaceState);
}
