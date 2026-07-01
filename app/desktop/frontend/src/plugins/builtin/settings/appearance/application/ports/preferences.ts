export type Theme = string;

export interface CustomTheme {
  bg: string;
  fg: string;
}

export interface AppearancePreferencesPort {
  useTheme(): Theme;
  useSetTheme(): (theme: Theme) => void;
  useAccent(): string;
  useSetAccent(): (accent: string) => void;
  useCustomTheme(): CustomTheme;
  useSetCustomTheme(): (patch: Partial<CustomTheme>) => void;
  useContrast(): number;
  useSetContrast(): (contrast: number) => void;
  useUiFont(): string;
  useCodeFont(): string;
  useFontSize(): number | null;
  useFontSmoothing(): boolean;
  useSetUiFont(): (font: string) => void;
  useSetCodeFont(): (font: string) => void;
  useSetFontSize(): (size: number | null) => void;
  useSetFontSmoothing(): (on: boolean) => void;
  useRadiusScale(): number;
  useMotionScale(): number;
  useSetRadiusScale(): (scale: number) => void;
  useSetMotionScale(): (scale: number) => void;
}

let port: AppearancePreferencesPort | null = null;

export function configureAppearancePreferencesPort(next: AppearancePreferencesPort): void {
  port = next;
}

export function appearancePreferences(): AppearancePreferencesPort {
  if (!port) throw new Error("Appearance preferences port is not configured");
  return port;
}
