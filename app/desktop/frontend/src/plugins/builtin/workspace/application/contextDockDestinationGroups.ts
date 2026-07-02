import type { ContextDockDestinationScope, ContextDockDestinationSpec } from "@/plugins/sdk";

export interface ContextDockDestinationGroup {
  id: ContextDockDestinationScope;
  title: string;
  destinations: ContextDockDestinationSpec[];
}

const groupOrder: Array<{ id: ContextDockDestinationScope; title: string }> = [
  { id: "workspace", title: "contextDock.group.workspace" },
  { id: "run", title: "contextDock.group.run" },
  { id: "session", title: "contextDock.group.session" },
];

export function groupContextDockDestinations(
  destinations: ContextDockDestinationSpec[],
): ContextDockDestinationGroup[] {
  return groupOrder
    .map((group) => ({
      ...group,
      destinations: destinations
        .filter((destination) => destination.placement === "context-dock")
        .filter((destination) => destination.scope === group.id)
        .sort((a, b) => (a.order ?? 100) - (b.order ?? 100)),
    }))
    .filter((group) => group.destinations.length > 0);
}
