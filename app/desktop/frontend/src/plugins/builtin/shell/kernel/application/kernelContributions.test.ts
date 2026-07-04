import { describe, expect, it } from "vitest";
import { kernelChatSlot, kernelSettingsView, kernelSidebarSlot } from "./kernelContributions";

function Component() {
  return null;
}

describe("kernelChatSlot", () => {
  it("projects the chat component into the main layout slot", () => {
    expect(kernelChatSlot(Component)).toEqual({
      id: "chat",
      order: 0,
      component: Component,
    });
  });
});

describe("kernelSidebarSlot", () => {
  it("projects the sidebar component into the sidebar layout slot", () => {
    expect(kernelSidebarSlot(Component)).toEqual({
      id: "sidebar",
      order: 0,
      component: Component,
    });
  });
});

describe("kernelSettingsView", () => {
  it("projects the settings component into the workspace view spec", () => {
    expect(kernelSettingsView(Component)).toEqual({
      id: "settings",
      title: "settings.title",
      icon: "settings",
      component: Component,
    });
  });
});
