import { describe, expect, it } from "vitest";
import {
  DEFAULT_ACCENTS,
  defaultAccentCommands,
  defaultMessageRoles,
  defaultStaticCommands,
  defaultWorkspaceViewCommands,
} from "./defaultContributions";

describe("DEFAULT_ACCENTS", () => {
  it("keeps accent ids stable and ordered for the appearance picker", () => {
    expect(DEFAULT_ACCENTS.map((accent) => accent.id)).toEqual(["blue", "green", "pink", "orange"]);
    expect(DEFAULT_ACCENTS.map((accent) => accent.order)).toEqual([0, 1, 2, 3]);
  });

  it("uses distinct light and dark values for every built-in accent", () => {
    expect(DEFAULT_ACCENTS.every((accent) => accent.light && accent.light !== accent.dark)).toBe(
      true,
    );
  });
});

describe("defaultMessageRoles", () => {
  it("projects translated display names into the three built-in message roles", () => {
    const roles = defaultMessageRoles((key) => `t:${key}`);

    expect(roles).toEqual([
      {
        id: "user",
        displayName: "t:role.user",
        icon: "user",
        avatarVariant: "msg-user",
      },
      {
        id: "assistant",
        displayName: "t:role.assistant",
        icon: "spark",
        avatarVariant: "msg-agent",
      },
      {
        id: "system",
        displayName: "t:role.system",
        icon: "shield",
        avatarVariant: "msg-agent",
      },
    ]);
  });
});

describe("defaultStaticCommands", () => {
  it("projects default command metadata in stable palette order", () => {
    const run = () => {};
    const commands = defaultStaticCommands((key) => `t:${key}`, {
      toggleSidebar: run,
      toggleTheme: run,
      newChat: run,
      closeSessionOrView: run,
      focusComposer: run,
    });

    expect(commands.map((command) => command.id)).toEqual([
      "view.toggle-sidebar",
      "settings.toggle-theme",
      "chat.new",
      "chat.close-session",
      "composer.focus",
    ]);
    expect(commands.map((command) => command.combo)).toEqual([
      "Mod+B",
      "Mod+Shift+L",
      "Mod+N",
      "Mod+W",
      "Mod+L",
    ]);
    expect(commands.map((command) => command.label)).toEqual([
      "t:command.toggleSidebar",
      "t:command.toggleTheme",
      "t:command.newChat",
      "t:command.closeSession",
      "t:command.focusComposer",
    ]);
  });
});

describe("defaultWorkspaceViewCommands", () => {
  it("mirrors workspace views into ordered palette commands", () => {
    function View() {
      return null;
    }

    const opened: unknown[] = [];
    const commands = defaultWorkspaceViewCommands(
      (key, values) => `${key}:${values?.title ?? ""}`,
      [
        { id: "late", title: "Late", icon: "clock", order: 20, component: View },
        { id: "early", title: "Early", icon: "spark", order: 0, component: View },
      ],
      (view) => opened.push(view),
    );

    expect(commands.map((command) => command.id)).toEqual(["view.open.early", "view.open.late"]);
    expect(commands[0]).toMatchObject({
      label: "command.viewPrefix:Early:",
      icon: "spark",
      group: "View",
      order: 10,
      keywords: ["open", "show", "early"],
      when: 'mainView != "early"',
    });

    void commands[0]!.run();

    expect(opened).toEqual([{ id: "early", title: "Early", icon: "spark" }]);
  });
});

describe("defaultAccentCommands", () => {
  it("mirrors theme accents into ordered palette commands", () => {
    const applied: string[] = [];
    const commands = defaultAccentCommands(
      (key, values) => `${key}:${values?.name ?? ""}`,
      [
        { id: "z", label: "Zed", dark: "#000", order: 9 },
        { id: "a", label: "Amber", dark: "#fff", order: 1 },
      ],
      (accent) => applied.push(accent),
    );

    expect(commands.map((command) => command.id)).toEqual(["theme.accent.a", "theme.accent.z"]);
    expect(commands[0]).toMatchObject({
      label: "command.accentPrefix:Amber",
      icon: "spark",
      group: "Theme",
      order: 10,
    });

    void commands[0]!.run();

    expect(applied).toEqual(["#fff"]);
  });
});
