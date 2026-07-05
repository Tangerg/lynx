import type { LayoutSlotSpec, ShortcutSpec } from "@/plugins/sdk";

export type Translate = (key: string) => string;

export function chatSearchOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "chat-search",
    order: 50,
    component,
  };
}

export function chatSearchShortcut(t: Translate, openSearch: () => void): ShortcutSpec {
  return {
    key: "Mod+F",
    description: t("chatSearch.shortcutDesc"),
    // Users usually trigger chat search while focus is still in the composer.
    allowInInputs: true,
    handler: (event) => {
      event.preventDefault();
      openSearch();
    },
  };
}
