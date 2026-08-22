import type { AccentSpec, CommandSpec, MessageRoleSpec } from "@/plugins/sdk";

export type CommandRun = CommandSpec["run"];

export interface DefaultCommandRuns {
  toggleSidebar: CommandRun;
  toggleDock: CommandRun;
  toggleTheme: CommandRun;
  newChat: CommandRun;
  closeFocused: CommandRun;
  focusComposer: CommandRun;
  historyBack: CommandRun;
  historyForward: CommandRun;
}

// The tool-window accent set. Each `dark` value is the hue as the language states
// it; `light` is the same hue pulled down until it clears 4.5:1 as text on the
// light scheme's chrome — a saturated mid-tone that reads as a fill on near-black
// reads as a highlighter pen on near-white.
export const DEFAULT_ACCENTS: AccentSpec[] = [
  {
    id: "blue",
    label: "Blue",
    dark: "#3574f0",
    light: "#2b5fd0",
    order: 0,
  },
  {
    id: "purple",
    label: "Purple",
    dark: "#7f52ff",
    light: "#6d3ff0",
    order: 1,
  },
  {
    id: "green",
    label: "Green",
    dark: "#21a179",
    light: "#177659",
    order: 2,
  },
  {
    id: "orange",
    label: "Orange",
    dark: "#e8590c",
    light: "#b34208",
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
      combo: "Mod+B",
      run: runs.toggleSidebar,
    },
    {
      id: "view.toggle-dock",
      label: "command.toggleDock",
      combo: "Mod+Shift+B",
      run: runs.toggleDock,
    },
    {
      id: "settings.toggle-theme",
      label: "command.toggleTheme",
      combo: "Mod+Shift+L",
      run: runs.toggleTheme,
    },
    {
      id: "chat.new",
      label: "command.newChat",
      combo: "Mod+N",
      run: runs.newChat,
    },
    {
      // Closes the dock's view if one is open, otherwise leaves the session —
      // which stays in the Work Index. "Close chat" implied it was going away.
      id: "workspace.close-focused",
      label: "command.closeFocused",
      combo: "Mod+W",
      run: runs.closeFocused,
    },
    {
      id: "composer.focus",
      label: "command.focusComposer",
      combo: "Mod+L",
      run: runs.focusComposer,
    },
    // The window has no address bar and no back button, so these two are the
    // only way to reach the history the location now records.
    {
      id: "history.back",
      label: "command.historyBack",
      combo: "Mod+[",
      run: runs.historyBack,
    },
    {
      id: "history.forward",
      label: "command.historyForward",
      combo: "Mod+]",
      run: runs.historyForward,
    },
  ];
}
