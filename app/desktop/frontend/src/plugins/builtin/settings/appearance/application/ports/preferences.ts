import type { AccentTint, ColorThemeId } from "@/lib/appearance";
import type { UiDensity } from "@/lib/density";
import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface CustomTheme {
  bg: string;
  fg: string;
}

export interface AppearancePreferencesPort {
  useTheme(): ColorThemeId;
  useSetTheme(): (theme: ColorThemeId) => void;
  useAccent(): string;
  useSetAccent(): (accent: string) => void;
  useCustomTheme(): CustomTheme;
  useSetCustomTheme(): (patch: Partial<CustomTheme>) => void;
  useContrast(): number;
  useSetContrast(): (contrast: number) => void;
  useAccentTint(): AccentTint;
  useSetAccentTint(): (tint: AccentTint) => void;
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
  useDensity(): UiDensity;
  useSetDensity(): (density: UiDensity) => void;
  useSetRadiusScale(): (scale: number) => void;
  useSetMotionScale(): (scale: number) => void;
}

const port = createSingletonPort<AppearancePreferencesPort>(
  "Appearance preferences port is not configured",
);

export const configureAppearancePreferencesPort = port.configure;
export const appearancePreferences = port.get;
