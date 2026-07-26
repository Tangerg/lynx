// The dock's tab strip, as the shell that draws it reads it: the destinations
// that asked to be pinned, resolved against the views actually registered.
//
// Which views get a chip is a contribution (CONTEXT_DOCK_DESTINATION), not a
// constant in the chrome — the shell used to hold the list of ids itself, so a
// plugin could add a dock destination but never a way to reach it in one click.

export { useContextDockPinned } from "../application/useContextDockLauncher";
export type { ContextDockLauncherItem } from "../application/contextDockDestinationGroups";
