import { describe, expect, it } from "vitest";
import * as workspaceViews from "./workspace/workspace-views";
import contextDockDestinations from "./workspace/context-dock";
import diagnostics from "./workspace/diagnostics";
import { loadPlugin } from "@/plugins/sdk";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { CONTEXT_DOCK_DESTINATION, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

// A composition invariant, so it lives with the manifest rather than inside one
// context: destinations are contributed by one plugin and the views they name by
// several others, and it is the assembled set that has to agree. Read off the
// registry — the same data the launcher and the dock's tab strip read.
describe("assembled context dock destinations", () => {
  async function assemble() {
    await Promise.all(
      [...Object.values(workspaceViews), diagnostics, contextDockDestinations].map((plugin) =>
        loadPlugin(plugin),
      ),
    );
    return {
      destinations: lookupExtensionPoint(CONTEXT_DOCK_DESTINATION),
      views: new Map(lookupExtensionPoint(WORKSPACE_VIEW).map((view) => [view.id, view])),
    };
  }

  // A destination whose viewId no longer resolves would render as a title-less
  // ghost (resolveContextDockItems drops it), so the launcher would silently
  // lose an entry.
  it("every destination names a registered view", async () => {
    const { destinations, views } = await assemble();

    const missing = destinations
      .map((destination) => destination.viewId)
      .filter((viewId) => !views.has(viewId));

    expect(missing).toEqual([]);
  });

  // Every destination opens in the dock, so a destination that cannot live there
  // is a one-way trip: the view would have no "open in the dock" affordance to
  // get back with.
  it("every destination's view can sit in the dock", async () => {
    const { destinations, views } = await assemble();

    const notSplittable = destinations
      .map((destination) => destination.viewId)
      .filter((viewId) => views.get(viewId)?.splittable !== true);

    expect(notSplittable).toEqual([]);
  });
});
