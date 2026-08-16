import { describe, expect, it } from "vitest";
import * as workspaceViews from "./workspace/workspace-views";
import contextDockDestinations from "./workspace/context-dock";
import diagnostics from "./workspace/diagnostics";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { CONTEXT_DOCK_DESTINATION, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// A composition invariant, so it lives with the manifest rather than inside one
// context: destinations are contributed by one plugin and the views they name by
// several others, and it is the assembled set that has to agree. Read off the
// registry — the same data the dock's add-panel menu reads.
describe("assembled context dock destinations", () => {
  async function assemble() {
    // One kernel holding all of them: the invariant is about the ASSEMBLED set,
    // and each call stands up a fresh Host.
    await loadPluginsForTest(
      ...Object.values(workspaceViews),
      diagnostics,
      contextDockDestinations,
    );
    return {
      destinations: lookupExtensionPoint(CONTEXT_DOCK_DESTINATION),
      views: new Map(lookupExtensionPoint(WORKSPACE_VIEW).map((view) => [view.id, view])),
    };
  }

  // A destination whose viewId no longer resolves would render as a title-less
  // ghost (resolveContextDockItems drops it), so the menu would silently
  // lose an entry.
  it("every destination names a registered view", async () => {
    const { destinations, views } = await assemble();

    const missing = destinations
      .map((destination) => destination.viewId)
      .filter((viewId) => !views.has(viewId));

    expect(missing).toEqual([]);
  });

  // The reverse direction, which nothing checked: a REGISTERED VIEW with no
  // destination is unreachable. It renders correctly, it can be photographed by a
  // fixture that registers a destination by hand, and in the product it never
  // appears in the add-panel menu at all — which is how the Inbox and Tool stats
  // views shipped registered and unreachable. Assert the whole set, so a view
  // added without a destination fails here instead of quietly going missing.
  it("every registered view is reachable from the dock", async () => {
    const { destinations, views } = await assemble();
    const reachable = new Set(destinations.map((destination) => destination.viewId));

    const unreachable = [...views.keys()].filter((viewId) => !reachable.has(viewId));

    expect(unreachable).toEqual([]);
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
