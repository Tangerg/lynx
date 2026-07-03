import { describe, expect, it } from "vitest";
import * as workspaceViews from "../workspace-views";
import { loadPlugin } from "@/plugins/sdk";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { builtinContextDockDestinations } from "./contextDockDestinations";

// Guard against silent drift: a builtin dock destination whose viewId no longer
// resolves to a registered workspace view would render as a title-less ghost
// (resolveContextDockItems drops it). The view owns title/icon/component, so
// every destination must point at a real view.
describe("builtin context dock destinations", () => {
  it("every destination references a registered workspace view", async () => {
    await Promise.all(Object.values(workspaceViews).map((plugin) => loadPlugin(plugin)));
    const viewIds = new Set(lookupExtensionPoint(WORKSPACE_VIEW).map((view) => view.id));

    const missing = builtinContextDockDestinations
      .map((destination) => destination.viewId)
      .filter((viewId) => !viewIds.has(viewId));

    expect(missing).toEqual([]);
  });
});
