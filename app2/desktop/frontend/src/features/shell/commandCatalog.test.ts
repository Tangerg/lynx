import { afterEach, describe, expect, it } from "vitest";

import {
  ariaKeyShortcuts,
  commandByID,
  commandCatalog,
  isEditableCommandTarget,
  matchesCommandShortcut,
  shortcutTokens,
} from "./commandCatalog";

const originalPlatform = Object.getOwnPropertyDescriptor(navigator, "platform");

afterEach(() => {
  if (originalPlatform === undefined)
    delete (navigator as { platform?: string }).platform;
  else Object.defineProperty(navigator, "platform", originalPlatform);
});

describe("shell command catalog", () => {
  it("has finite, collision-free identities and shortcuts", () => {
    expect(commandCatalog.map((command) => command.id)).toEqual([
      "session.new",
      "session.search",
      "narrative.search",
      "settings.open",
      "workspace.close",
    ]);
    const shortcuts = commandCatalog.map(({ shortcut }) =>
      JSON.stringify([
        shortcut.mod,
        shortcut.alt,
        shortcut.shift,
        shortcut.code,
      ]),
    );
    expect(new Set(shortcuts).size).toBe(shortcuts.length);
    expect(commandByID("settings.open").label).toBe("command.openSettings");
  });

  it("matches platform modifier semantics and refuses IME/repeat input", () => {
    const shortcut = commandByID("session.new").shortcut;
    Object.defineProperty(navigator, "platform", {
      configurable: true,
      value: "Win32",
    });
    expect(
      matchesCommandShortcut(shortcut, key({ code: "KeyN", ctrlKey: true })),
    ).toBe(true);
    expect(
      matchesCommandShortcut(shortcut, key({ code: "KeyN", metaKey: true })),
    ).toBe(false);

    Object.defineProperty(navigator, "platform", {
      configurable: true,
      value: "MacIntel",
    });
    expect(
      matchesCommandShortcut(shortcut, key({ code: "KeyN", metaKey: true })),
    ).toBe(true);
    expect(
      matchesCommandShortcut(
        shortcut,
        key({ code: "KeyN", metaKey: true, repeat: true }),
      ),
    ).toBe(false);
    expect(
      matchesCommandShortcut(
        shortcut,
        key({ code: "KeyN", metaKey: true, isComposing: true }),
      ),
    ).toBe(false);
    expect(
      matchesCommandShortcut(
        shortcut,
        key({ code: "KeyN", metaKey: true, keyCode: 229 }),
      ),
    ).toBe(false);
  });

  it("recognizes editable targets and exposes accessible shortcut labels", () => {
    const input = document.createElement("input");
    const editable = document.createElement("div");
    editable.setAttribute("contenteditable", "true");
    const child = document.createElement("span");
    editable.append(child);
    expect(isEditableCommandTarget(input)).toBe(true);
    expect(isEditableCommandTarget(child)).toBe(true);
    expect(isEditableCommandTarget(document.createElement("button"))).toBe(
      false,
    );
    expect(shortcutTokens(commandByID("settings.open").shortcut, true)).toEqual(
      ["⌘", ","],
    );
    expect(ariaKeyShortcuts(commandByID("settings.open").shortcut)).toBe(
      "Meta+, Control+,",
    );
  });
});

function key(init: KeyboardEventInit & { keyCode?: number }): KeyboardEvent {
  const event = new KeyboardEvent("keydown", init);
  if (init.keyCode !== undefined) {
    Object.defineProperty(event, "keyCode", { value: init.keyCode });
  }
  return event;
}
