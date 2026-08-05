import type { LayoutSlotSpec, ShortcutSpec } from "@/plugins/sdk";

export function sessionSearchOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "session-search",
    order: 10,
    component,
  };
}

export function sessionSearchShortcut(toggle: () => void): ShortcutSpec {
  return {
    key: "Mod+K",
    description: "shortcut.sessionSearch",
    // ⌘K is what a person reaches for mid-sentence in the composer, so it has to
    // survive an input having focus.
    allowInInputs: true,
    handler: (event) => {
      event.preventDefault();
      toggle();
    },
  };
}
