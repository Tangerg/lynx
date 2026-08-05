import type { CommandSpec } from "@/plugins/sdk";
import { describe, expect, it, vi } from "vitest";
import { globalCommandShortcuts, workspaceEscapeShortcut } from "./globalKeymap";

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
  it("closes the workspace view", () => {
    const closeActiveWorkspaceView = vi.fn(() => true);
    const shortcut = workspaceEscapeShortcut((k: string) => k, closeActiveWorkspaceView);

    expect(shortcut).toMatchObject({
      key: "Escape",
      description: "shortcut.closeWorkspaceView",
      allowInInputs: false,
    });

    // The palette used to own Escape while it was open, so this went through a
    // guard that asked first. One meaning now, so the handler just closes.
    shortcut.handler(new KeyboardEvent("keydown"));
    expect(closeActiveWorkspaceView).toHaveBeenCalledOnce();
  });
});
