import type { CommandSpec } from "@/plugins/sdk";
import { describe, expect, it, vi } from "vitest";
import {
  globalCommandShortcuts,
  handleWorkspaceEscape,
  workspaceEscapeShortcut,
} from "./globalKeymap";

const command = (patch: Partial<CommandSpec> & Pick<CommandSpec, "id">): CommandSpec => ({
  label: patch.id,
  run: () => undefined,
  ...patch,
});

describe("globalCommandShortcuts", () => {
  it("binds only catalog commands with combos", () => {
    const shortcuts = globalCommandShortcuts((id) =>
      id === "chat.new"
        ? command({ id, label: "New Chat", combo: "Mod+N" })
        : command({ id, label: id }),
    );

    expect(shortcuts).toHaveLength(1);
    expect(shortcuts[0]).toMatchObject({
      key: "Mod+N",
      description: "New Chat",
      allowInInputs: true,
    });
  });

  it("resolves the command again when the shortcut fires", () => {
    const initialRun = vi.fn();
    const replacementRun = vi.fn();
    let active = command({ id: "chat.new", label: "New Chat", combo: "Mod+N", run: initialRun });
    const shortcuts = globalCommandShortcuts((id) => (id === "chat.new" ? active : undefined));
    active = command({ id: "chat.new", label: "New Chat", combo: "Mod+N", run: replacementRun });
    const event = { preventDefault: vi.fn() } as unknown as KeyboardEvent;

    shortcuts[0]?.handler(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(initialRun).not.toHaveBeenCalled();
    expect(replacementRun).toHaveBeenCalledOnce();
  });
});

describe("workspaceEscapeShortcut", () => {
  it("exposes the workspace close binding", () => {
    const shortcut = workspaceEscapeShortcut({
      isPaletteOpen: () => false,
      closeActiveWorkspaceView: () => false,
    });

    expect(shortcut).toMatchObject({
      key: "Escape",
      description: "Close workspace view",
      allowInInputs: false,
    });
  });

  it("does not close the workspace while the palette owns Escape", () => {
    const closeActiveWorkspaceView = vi.fn();

    expect(
      handleWorkspaceEscape({
        isPaletteOpen: () => true,
        closeActiveWorkspaceView,
      }),
    ).toBe(false);
    expect(closeActiveWorkspaceView).not.toHaveBeenCalled();
  });

  it("delegates Escape to the workspace when the palette is closed", () => {
    const closeActiveWorkspaceView = vi.fn(() => true);

    expect(
      handleWorkspaceEscape({
        isPaletteOpen: () => false,
        closeActiveWorkspaceView,
      }),
    ).toBe(true);
    expect(closeActiveWorkspaceView).toHaveBeenCalledOnce();
  });
});
