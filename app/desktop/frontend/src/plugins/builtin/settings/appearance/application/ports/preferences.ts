import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<AppearancePreferencesPort>(
  "Appearance preferences port is not configured",
);

export const configureAppearancePreferencesPort = port.configure;
export const appearancePreferences = port.get;
