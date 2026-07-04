import type { MessageRoleSpec, ThemeAccentSpec } from "@/plugins/sdk";

export type Translate = (key: string) => string;

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
