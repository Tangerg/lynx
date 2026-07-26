import type { CommandSpec, LayoutSlotSpec } from "@/plugins/sdk";

export function chatSearchOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "chat-search",
    order: 50,
    component,
  };
}

/** A command, not a bare shortcut: the global keymap turns its combo into the
 *  binding, and being a command is what makes it findable in the palette. As a
 *  shortcut-only feature, ⌘F was discoverable exactly by knowing it existed. */
export function chatSearchCommand(openSearch: () => void): CommandSpec {
  return {
    id: "chat.search",
    label: "command.chatSearch",
    icon: "search",
    group: "command.group.chat",
    keywords: ["find", "search", "conversation"],
    order: 2,
    combo: "Mod+F",
    run: openSearch,
  };
}
