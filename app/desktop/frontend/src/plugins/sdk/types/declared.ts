// Shapes a lazily-activated plugin declares ahead of its setup: the same specs
// the activated plugin will contribute, minus the component the kernel stands a
// placeholder in for.

import type { SettingsPaneSpec, WorkspaceViewSpec } from "./workspace";

export type ContributedView = Omit<WorkspaceViewSpec, "component">;
export type ContributedSettingsPane = Omit<SettingsPaneSpec, "component">;
