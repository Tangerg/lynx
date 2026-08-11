import { describe, expect, it } from "vitest";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { loadPlugins, lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { toolPreviewPlugins } from "./index";

describe("built-in tool rendering composition", () => {
  it("installs one independent preview component for every known tool", async () => {
    const loaded = await loadPlugins(toolPreviewPlugins);
    expect(loaded.every((result) => result.kind === "loaded")).toBe(true);

    const components = Object.keys(TOOL_ICON_BY_NAME).map((name) => {
      const component = lookupExtensionByKey(TOOL_PREVIEW, name);
      expect(component, `${name} preview`).toBeDefined();
      return component;
    });

    expect(new Set(components).size).toBe(components.length);
  });
});
