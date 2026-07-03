import { resolveScheme } from "@/plugins/sdk/selectors/theme";

export type ThemeScheme = "dark" | "light";

export function resolveThemeScheme(themeId: string): ThemeScheme {
  return resolveScheme(themeId);
}

export function isLightTheme(themeId: string): boolean {
  return resolveThemeScheme(themeId) === "light";
}
