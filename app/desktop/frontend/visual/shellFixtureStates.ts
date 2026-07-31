export const VISUAL_WORK_INDEX_STATES = ["populated", "empty", "loading", "error"] as const;

export type VisualWorkIndexState = (typeof VISUAL_WORK_INDEX_STATES)[number];
export type VisualShellTheme = "light" | "dark";
