import type {
  CommandSpec,
  MessageRoleSpec,
  ThemeAccentSpec,
  WorkspaceViewSpec,
} from "@/plugins/sdk";

export type CommandRun = CommandSpec["run"];

export interface DefaultCommandRuns {
  toggleSidebar: CommandRun;
  toggleTheme: CommandRun;
  newChat: CommandRun;
  closeFocused: CommandRun;
  focusComposer: CommandRun;
}

/** The two placements a view can be opened into. Which one a command uses is the
 *  view's own `splittable`, not the caller's guess — the palette used to send
 *  everything to the full card, so the same destination behaved differently
 *  depending on whether you reached it from the palette or the dock. */
export interface WorkspaceViewOpeners {
  openInDock: (id: WorkspaceViewSpec["id"]) => void;
  openFull: (id: WorkspaceViewSpec["id"]) => void;
}

export type AccentSetter = (accent: ThemeAccentSpec["dark"]) => void;

export const DEFAULT_ACCENTS: ThemeAccentSpec[] = [
  {
    id: "blue",
    label: "Blue",
    dark: "#6c97ff",
    light: "#2563eb",
    order: 0,
  },
  {
    id: "green",
    label: "Green",
    dark: "#1ed760",
    light: "#169c46",
    order: 1,
  },
  {
    id: "pink",
    label: "Pink",
    dark: "#e07acc",
    light: "#a823a3",
    order: 2,
  },
  {
    id: "orange",
    label: "Orange",
    dark: "#ffa42b",
    light: "#d97706",
    order: 3,
  },
];

export function defaultMessageRoles(): MessageRoleSpec[] {
  return [
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
  ];
}

export function defaultStaticCommands(runs: DefaultCommandRuns): CommandSpec[] {
  return [
    {
      id: "view.toggle-sidebar",
      label: "command.toggleSidebar",
      icon: "panel-l",
      group: "command.group.view",
      keywords: ["collapse", "expand"],
      order: 0,
      combo: "Mod+B",
      run: runs.toggleSidebar,
    },
    {
      id: "settings.toggle-theme",
      label: "command.toggleTheme",
      icon: "moon",
      group: "command.group.theme",
      order: 0,
      combo: "Mod+Shift+L",
      run: runs.toggleTheme,
    },
    {
      id: "chat.new",
      label: "command.newChat",
      icon: "plus",
      group: "command.group.chat",
      keywords: ["session", "open"],
      order: 0,
      combo: "Mod+N",
      run: runs.newChat,
    },
    {
      // Closes the dock's view if one is open, otherwise leaves the session —
      // which stays in the Work Index. "Close chat" implied it was going away.
      id: "workspace.close-focused",
      label: "command.closeFocused",
      description: "command.closeFocused.desc",
      icon: "x",
      group: "command.group.chat",
      keywords: ["dismiss", "leave"],
      order: 1,
      combo: "Mod+W",
      run: runs.closeFocused,
    },
    {
      id: "composer.focus",
      label: "command.focusComposer",
      icon: "edit",
      group: "command.group.composer",
      keywords: ["input", "write"],
      order: 0,
      combo: "Mod+L",
      run: runs.focusComposer,
    },
  ];
}

export function defaultWorkspaceViewCommands(
  views: WorkspaceViewSpec[],
  open: WorkspaceViewOpeners,
): CommandSpec[] {
  return [...views]
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    .map((view) => ({
      id: `view.open.${view.id}`,
      // The view's own title key; the group column beside it already says "View",
      // which is what the old "View: {title}" label was repeating.
      label: view.title,
      icon: view.icon,
      group: "command.group.view",
      order: 10,
      keywords: ["open", "show", view.id],
      when: `mainView != "${view.id}"`,
      run: () => (view.splittable ? open.openInDock(view.id) : open.openFull(view.id)),
    }));
}

export function defaultAccentCommands(
  accents: ThemeAccentSpec[],
  setAccent: AccentSetter,
): CommandSpec[] {
  return [...accents]
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    .map((accent) => ({
      id: `theme.accent.${accent.id}`,
      // A colour's own name — a proper noun, not copy, so it passes through the
      // catalog unchanged. "accent" stays findable through the keywords.
      label: accent.label,
      icon: "spark",
      group: "command.group.theme",
      keywords: ["accent", "color", "colour"],
      order: 10,
      run: () => setAccent(accent.dark),
    }));
}
