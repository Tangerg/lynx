import type {
  ContextDockDestinationScope,
  ContextDockDestinationSpec,
  WorkspaceViewSpec,
} from "@/plugins/sdk";

// A dock destination joined with its WorkspaceViewSpec — the view supplies
// title/icon, the spec supplies scope/order. This is the Context Dock
// launcher's read model (what the launcher renders and opens).
export interface ContextDockLauncherItem {
  viewId: string;
  title: string;
  icon?: string;
  scope: ContextDockDestinationScope;
  order?: number;
}

export interface ContextDockDestinationGroup {
  id: ContextDockDestinationScope;
  title: string;
  destinations: ContextDockLauncherItem[];
}

// Join destinations with the registered workspace views. A destination whose
// viewId no longer resolves (a plugin referencing a removed view) is dropped so
// the launcher never renders a title-less ghost; builtins are guarded by a test.
export function resolveContextDockItems(
  destinations: readonly ContextDockDestinationSpec[],
  views: readonly Pick<WorkspaceViewSpec, "id" | "title" | "icon">[],
): ContextDockLauncherItem[] {
  const byId = new Map(views.map((view) => [view.id, view]));
  const items: ContextDockLauncherItem[] = [];
  for (const destination of destinations) {
    const view = byId.get(destination.viewId);
    if (!view) continue;
    items.push({
      viewId: destination.viewId,
      title: view.title,
      icon: view.icon,
      scope: destination.scope,
      order: destination.order,
    });
  }
  return items;
}

const groupOrder: Array<{ id: ContextDockDestinationScope; title: string }> = [
  { id: "workspace", title: "contextDock.group.workspace" },
  { id: "run", title: "contextDock.group.run" },
  { id: "session", title: "contextDock.group.session" },
];

export function groupContextDockDestinations(
  items: ContextDockLauncherItem[],
): ContextDockDestinationGroup[] {
  return groupOrder
    .map((group) => ({
      ...group,
      destinations: items
        .filter((item) => item.scope === group.id)
        .sort(
          (a, b) => (a.order ?? Number.MAX_SAFE_INTEGER) - (b.order ?? Number.MAX_SAFE_INTEGER),
        ),
    }))
    .filter((group) => group.destinations.length > 0);
}
