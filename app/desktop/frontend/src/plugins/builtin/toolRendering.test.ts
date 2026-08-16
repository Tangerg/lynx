import { describe, expect, it } from "vitest";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { toolPreviewPlugins } from "./index";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

describe("built-in tool rendering composition", () => {
  it("installs one independent preview component for every known tool", async () => {
    // One transaction: a preview plugin that fails to install takes the whole
    // boot down, which is the assertion.
    await loadPluginsForTest(...toolPreviewPlugins);

    const components = Object.keys(TOOL_ICON_BY_NAME).map((name) => {
      const component = lookupExtensionByKey(TOOL_PREVIEW, name);
      expect(component, `${name} preview`).toBeDefined();
      return component;
    });

    expect(new Set(components).size).toBe(components.length);
  });
});
