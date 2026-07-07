import type {
  CommandSpec,
  MessageRoleSpec,
  ThemeAccentSpec,
  WorkspaceViewSpec,
} from "@/plugins/sdk";

export type Translate = (key: string, values?: Record<string, string>) => string;
export type CommandRun = CommandSpec["run"];

export interface DefaultCommandRuns {
  toggleSidebar: CommandRun;
  toggleTheme: CommandRun;
  newChat: CommandRun;
  closeSessionOrView: CommandRun;
  focusComposer: CommandRun;
}

export type WorkspaceViewOpener = (id: WorkspaceViewSpec["id"]) => void;

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

export function defaultMessageRoles(t: Translate): MessageRoleSpec[] {
  return [
    {
      id: "user",
      displayName: t("role.user"),
      icon: "user",
      avatarVariant: "msg-user",
    },
    {
      id: "assistant",
      displayName: t("role.assistant"),
      icon: "spark",
      avatarVariant: "msg-agent",
    },
    {
      id: "system",
      displayName: t("role.system"),
      icon: "shield",
      avatarVariant: "msg-agent",
    },
  ];
}

export function defaultStaticCommands(t: Translate, runs: DefaultCommandRuns): CommandSpec[] {
  return [
    {
      id: "view.toggle-sidebar",
      label: t("command.toggleSidebar"),
      icon: "panel-l",
      group: "View",
      keywords: ["collapse", "expand"],
      order: 0,
      combo: "Mod+B",
      run: runs.toggleSidebar,
    },
    {
      id: "settings.toggle-theme",
      label: t("command.toggleTheme"),
      icon: "moon",
      group: "Theme",
      order: 0,
      combo: "Mod+Shift+L",
      run: runs.toggleTheme,
    },
    {
      id: "chat.new",
      label: t("command.newChat"),
      icon: "plus",
      group: "Chat",
      keywords: ["session", "open"],
      order: 0,
      combo: "Mod+N",
      run: runs.newChat,
    },
    {
      id: "chat.close-session",
      label: t("command.closeSession"),
      icon: "x",
      group: "Chat",
      keywords: ["dismiss"],
      order: 1,
      combo: "Mod+W",
      run: runs.closeSessionOrView,
    },
    {
      id: "composer.focus",
      label: t("command.focusComposer"),
      icon: "edit",
      group: "Composer",
      keywords: ["input", "write"],
      order: 0,
      combo: "Mod+L",
      run: runs.focusComposer,
    },
  ];
}

export function defaultWorkspaceViewCommands(
  t: Translate,
  views: WorkspaceViewSpec[],
  openView: WorkspaceViewOpener,
): CommandSpec[] {
  return [...views]
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    .map((view) => ({
      id: `view.open.${view.id}`,
      label: t("command.viewPrefix", { title: t(view.title) }),
      icon: view.icon,
      group: "View",
      order: 10,
      keywords: ["open", "show", view.id],
      when: `mainView != "${view.id}"`,
      run: () => openView(view.id),
    }));
}

export function defaultAccentCommands(
  t: Translate,
  accents: ThemeAccentSpec[],
  setAccent: AccentSetter,
): CommandSpec[] {
  return [...accents]
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    .map((accent) => ({
      id: `theme.accent.${accent.id}`,
      label: t("command.accentPrefix", { name: accent.label }),
      icon: "spark",
      group: "Theme",
      order: 10,
      run: () => setAccent(accent.dark),
    }));
}
