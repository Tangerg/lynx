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
        .filter((destination) => destination.scope === group.id)
        .sort(
          (a, b) => (a.order ?? Number.MAX_SAFE_INTEGER) - (b.order ?? Number.MAX_SAFE_INTEGER),
        ),
    }))
    .filter((group) => group.destinations.length > 0);
}
