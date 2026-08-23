import type { MessageKey } from "../localization/Localization";

export type ShellCommandID =
  | "session.new"
  | "session.search"
  | "narrative.search"
  | "settings.open"
  | "workspace.close";

export type CommandScope = "global" | "session" | "workspace";

export interface CommandShortcut {
  code: "KeyN" | "KeyK" | "KeyF" | "Comma" | "Escape";
  mod?: boolean;
  shift?: boolean;
  alt?: boolean;
  allowInEditable: boolean;
}

export interface CommandDescriptor {
  id: ShellCommandID;
  label: MessageKey;
  scope: CommandScope;
  shortcut: CommandShortcut;
}

export const commandCatalog = [
  command("session.new", "command.newSession", "global", {
    code: "KeyN",
    mod: true,
    allowInEditable: true,
  }),
  command("session.search", "command.searchSessions", "global", {
    code: "KeyK",
    mod: true,
    allowInEditable: true,
  }),
  command("narrative.search", "command.searchConversation", "session", {
    code: "KeyF",
    mod: true,
    allowInEditable: true,
  }),
  command("settings.open", "command.openSettings", "global", {
    code: "Comma",
    mod: true,
    allowInEditable: true,
  }),
  command("workspace.close", "command.closeWorkspaceView", "workspace", {
    code: "Escape",
    allowInEditable: false,
  }),
] as const satisfies readonly CommandDescriptor[];

assertUniqueCatalog(commandCatalog);

export function commandByID(id: ShellCommandID): CommandDescriptor {
  const descriptor = commandCatalog.find((candidate) => candidate.id === id);
  if (descriptor === undefined) {
    throw new Error(`Unknown shell command: ${id}`);
  }
  return descriptor;
}

export function matchesCommandShortcut(
  shortcut: CommandShortcut,
  event: KeyboardEvent,
): boolean {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.isComposing ||
    event.keyCode === 229 ||
    event.code !== shortcut.code ||
    event.altKey !== Boolean(shortcut.alt) ||
    event.shiftKey !== Boolean(shortcut.shift)
  ) {
    return false;
  }
  if (!shortcut.mod) return !event.metaKey && !event.ctrlKey;
  return isApplePlatform()
    ? event.metaKey && !event.ctrlKey
    : event.ctrlKey && !event.metaKey;
}

export function isEditableCommandTarget(target: EventTarget | null): boolean {
  return (
    target instanceof Element &&
    target.closest(
      'input, textarea, select, [contenteditable=""], [contenteditable="true"]',
    ) !== null
  );
}

export function shortcutTokens(
  shortcut: CommandShortcut,
  apple = isApplePlatform(),
): readonly string[] {
  const tokens: string[] = [];
  if (shortcut.mod) tokens.push(apple ? "⌘" : "Ctrl");
  if (shortcut.alt) tokens.push(apple ? "⌥" : "Alt");
  if (shortcut.shift) tokens.push(apple ? "⇧" : "Shift");
  tokens.push(keyLabel(shortcut.code));
  return tokens;
}

export function ariaKeyShortcuts(shortcut: CommandShortcut): string {
  const key = ariaKeyLabel(shortcut.code);
  const modifiers = [
    shortcut.alt ? "Alt" : undefined,
    shortcut.shift ? "Shift" : undefined,
  ].filter((value): value is string => value !== undefined);
  if (!shortcut.mod) return [...modifiers, key].join("+");
  return (
    ["Meta", ...modifiers, key].join("+") +
    " " +
    ["Control", ...modifiers, key].join("+")
  );
}

function command(
  id: ShellCommandID,
  label: MessageKey,
  scope: CommandScope,
  shortcut: CommandShortcut,
): CommandDescriptor {
  return { id, label, scope, shortcut };
}

function keyLabel(code: CommandShortcut["code"]): string {
  if (code === "Escape") return "Esc";
  if (code === "Comma") return ",";
  return code.slice(3);
}

function ariaKeyLabel(code: CommandShortcut["code"]): string {
  if (code === "Escape") return "Escape";
  if (code === "Comma") return ",";
  return code.slice(3).toLowerCase();
}

function isApplePlatform(): boolean {
  if (typeof navigator === "undefined") return false;
  return /Mac|iPhone|iPad|iPod/.test(navigator.platform);
}

function assertUniqueCatalog(catalog: readonly CommandDescriptor[]): void {
  const identities = new Set<ShellCommandID>();
  const shortcuts = new Set<string>();
  for (const descriptor of catalog) {
    if (identities.has(descriptor.id)) {
      throw new Error(`Duplicate shell command: ${descriptor.id}`);
    }
    identities.add(descriptor.id);
    const shortcut = shortcutIdentity(descriptor.shortcut);
    if (shortcuts.has(shortcut)) {
      throw new Error(`Duplicate shell shortcut: ${shortcut}`);
    }
    shortcuts.add(shortcut);
  }
}

function shortcutIdentity(shortcut: CommandShortcut): string {
  return [
    shortcut.mod ? "mod" : "",
    shortcut.alt ? "alt" : "",
    shortcut.shift ? "shift" : "",
    shortcut.code,
  ].join(":");
}
