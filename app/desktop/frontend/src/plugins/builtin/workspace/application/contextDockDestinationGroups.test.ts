import { describe, expect, it } from "vitest";
import {
  groupContextDockDestinations,
  resolveContextDockItems,
  type ContextDockLauncherItem,
} from "./contextDockDestinationGroups";

const item = (
  patch: Pick<ContextDockLauncherItem, "viewId" | "scope"> & Partial<ContextDockLauncherItem>,
): ContextDockLauncherItem => ({
  title: `title.${patch.viewId}`,
  ...patch,
});

describe("resolveContextDockItems", () => {
  it("joins destinations with view title/icon and drops unresolved viewIds", () => {
    const items = resolveContextDockItems(
      [
        { viewId: "search", scope: "workspace", order: 10 },
        { viewId: "ghost", scope: "workspace", order: 20 },
      ],
      [
        { id: "search", title: "workspace.view.title.search", icon: "search" },
        { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      ],
    );

    expect(items).toEqual([
      {
        viewId: "search",
        title: "workspace.view.title.search",
        icon: "search",
        scope: "workspace",
        order: 10,
      },
    ]);
  });
});

describe("groupContextDockDestinations", () => {
  it("groups items by workspace mental-model scope and sorts by order", () => {
    const groups = groupContextDockDestinations([
      item({ viewId: "timeline", scope: "session", order: 10 }),
      item({ viewId: "plan", scope: "run", order: 10 }),
      item({ viewId: "files", scope: "workspace", order: 20 }),
      item({ viewId: "search", scope: "workspace", order: 10 }),
    ]);

    expect(groups.map((group) => group.id)).toEqual(["workspace", "run", "session"]);
    expect(groups[0]?.destinations.map((d) => d.viewId)).toEqual(["search", "files"]);
    expect(groups[1]?.destinations.map((d) => d.viewId)).toEqual(["plan"]);
    expect(groups[2]?.destinations.map((d) => d.viewId)).toEqual(["timeline"]);
  });
});
