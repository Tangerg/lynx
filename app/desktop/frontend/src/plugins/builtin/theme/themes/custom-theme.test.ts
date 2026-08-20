import { beforeEach, describe, expect, it } from "vitest";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { useUiStore } from "@/state/uiStore";
import customTheme from "./custom-theme";

describe("custom theme contribution lifecycle", () => {
  beforeEach(() => {
    useUiStore.setState({
      accent: "#3574f0",
      contrast: 25,
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
    });
  });

  it("replaces its single contribution when appearance preferences change", async () => {
    await loadPluginsForTest(customTheme);

    expect(lookupExtensionPoint(COLOR_THEME)).toHaveLength(1);
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe("#3574f0");

    expect(() => useUiStore.getState().setAccent("#7f52ff")).not.toThrow();
    expect(lookupExtensionPoint(COLOR_THEME)).toHaveLength(1);
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe("#7f52ff");

    await resetKernelForTest();
    expect(() => useUiStore.getState().setAccent("#21a179")).not.toThrow();
  });
});
