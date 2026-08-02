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
  it("projects default command metadata in stable palette order", () => {
    const run = () => {};
    const commands = defaultStaticCommands({
      toggleSidebar: run,
      toggleTheme: run,
      newChat: run,
      closeFocused: run,
      focusComposer: run,
    });

    expect(commands.map((command) => command.id)).toEqual([
      "view.toggle-sidebar",
      "settings.toggle-theme",
      "chat.new",
      "workspace.close-focused",
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
      "command.toggleSidebar",
      "command.toggleTheme",
      "command.newChat",
      "command.closeFocused",
      "command.focusComposer",
    ]);
  });
});

describe("defaultWorkspaceViewCommands", () => {
  it("mirrors workspace views into ordered palette commands", () => {
    function View() {
      return null;
    }

    const inDock: string[] = [];
    const full: string[] = [];
    const commands = defaultWorkspaceViewCommands(
      [
        {
          id: "late",
          title: "workspace.view.title.late",
          icon: "clock",
          order: 20,
          component: View,
        },
        {
          id: "early",
          title: "workspace.view.title.early",
          icon: "spark",
          order: 0,
          splittable: true,
          component: View,
        },
      ],
      {
        openInDock: (view: string) => inDock.push(view),
        openFull: (view: string) => full.push(view),
      },
    );

    expect(commands.map((command) => command.id)).toEqual(["view.open.early", "view.open.late"]);
    // The view's own title key, carried through — the palette resolves it, and the
    // group column says "View" so the label doesn't repeat it.
    expect(commands[0]).toMatchObject({
      label: "workspace.view.title.early",
      icon: "spark",
      group: "command.group.view",
      order: 10,
      keywords: ["open", "show", "early"],
      when: 'mainView != "early"',
    });

    // Placement follows the view, not the caller: a view that can live in the dock
    // opens there, one that cannot takes the whole card.
    void commands[0]!.run();
    void commands[1]!.run();

    expect(inDock).toEqual(["early"]);
    expect(full).toEqual(["late"]);
  });
});

describe("defaultAccentCommands", () => {
  it("mirrors theme accents into ordered palette commands", () => {
    const applied: string[] = [];
    const commands = defaultAccentCommands(
      [
        { id: "z", label: "Zed", dark: "#000", order: 9 },
        { id: "a", label: "Amber", dark: "#fff", order: 1 },
      ],
      (accent: string) => applied.push(accent),
    );

    expect(commands.map((command) => command.id)).toEqual(["theme.accent.a", "theme.accent.z"]);
    expect(commands[0]).toMatchObject({
      label: "Amber",
      icon: "spark",
      group: "command.group.theme",
      order: 10,
    });

    void commands[0]!.run();

    expect(applied).toEqual(["#fff"]);
  });
});
