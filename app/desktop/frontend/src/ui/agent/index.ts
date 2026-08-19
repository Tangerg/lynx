export { AgentAppShell } from "./app-shell";
export { AgentActivityDisclosure } from "./activity-disclosure";
export { AgentComposerSurface } from "./composer-surface";
export { AgentComposerTopTraySurface } from "./composer-top-tray-surface";
export { AgentContentCard } from "./content-card";
export { AgentContextDock, AgentDockTabs, type AgentDockTab } from "./context-dock";
export { AgentRow } from "./navigation-row";
export { AgentStatusPill } from "./status-pill";
// AgentDrawerToggle is deliberately NOT re-exported: the shell mounts the one
// instance itself, and a second one anywhere else is a second collapse control.
export { AgentDockToggle, AgentSurfaceHeader } from "./surface-header";
export {
  AgentViewNavigator,
  AgentViewNavigatorToggle,
  AgentViewSplit,
  AgentWorkspaceView,
} from "./workspace-view";
export {
  AgentWorkIndexBody,
  AgentWorkIndexFooter,
  AgentWorkIndexGroupList,
  AgentWorkIndexSection,
} from "./work-index";
