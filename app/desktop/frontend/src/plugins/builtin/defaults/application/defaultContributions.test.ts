import { describe, expect, it } from "vitest";
import {
  DEFAULT_ACCENTS,
  defaultMessageRoles,
  defaultStaticCommands,
} from "./defaultContributions";

describe("DEFAULT_ACCENTS", () => {
  it("keeps accent ids stable and ordered for the appearance picker", () => {
    expect(DEFAULT_ACCENTS.map((accent) => accent.id)).toEqual([
      "blue",
      "purple",
      "green",
      "orange",
    ]);
    expect(DEFAULT_ACCENTS.map((accent) => accent.order)).toEqual([0, 1, 2, 3]);
  });

  it("uses distinct light and dark values for every built-in accent", () => {
    expect(DEFAULT_ACCENTS.every((accent) => accent.light && accent.light !== accent.dark)).toBe(
      true,
    );
  });
});

describe("defaultMessageRoles", () => {
  it("projects catalog keys into the three built-in message roles", () => {
    const roles = defaultMessageRoles();

    expect(roles).toEqual([
      {
        id: "user",
        displayName: "role.user",
        icon: "user",
        avatarVariant: "msg-user",
      },
      {
        id: "assistant",
        displayName: "role.assistant",
        icon: "spark",
        avatarVariant: "msg-agent",
      },
      {
        id: "system",
        displayName: "role.system",
        icon: "shield",
        avatarVariant: "msg-agent",
      },
    ]);
  });
});

describe("defaultStaticCommands", () => {
  it("projects the stable command ids, combos, and shortcut labels", () => {
    const run = () => {};
    const commands = defaultStaticCommands({
      toggleSidebar: run,
      toggleDock: run,
      toggleTheme: run,
      newChat: run,
      closeFocused: run,
      focusComposer: run,
      historyBack: run,
      historyForward: run,
    });

    expect(commands.map((command) => command.id)).toEqual([
      "view.toggle-sidebar",
      "view.toggle-dock",
      "settings.toggle-theme",
      "chat.new",
      "workspace.close-focused",
      "composer.focus",
      "history.back",
      "history.forward",
    ]);
    expect(commands.map((command) => command.combo)).toEqual([
      "Mod+B",
      "Mod+Shift+B",
      "Mod+Shift+L",
      "Mod+N",
      "Mod+W",
      "Mod+L",
      "Mod+[",
      "Mod+]",
    ]);
    expect(commands.map((command) => command.label)).toEqual([
      "command.toggleSidebar",
      "command.toggleDock",
      "command.toggleTheme",
      "command.newChat",
      "command.closeFocused",
      "command.focusComposer",
      "command.historyBack",
      "command.historyForward",
    ]);
  });
});
