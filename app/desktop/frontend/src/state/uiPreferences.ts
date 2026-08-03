import type { AccentTint } from "@/lib/appearance";
import type { ColorThemeId, VisualStyleId } from "@/lib/appearance";
import type { UiDensity } from "@/lib/density";

export interface CustomTheme {
  bg: string;
  fg: string;
}

export interface UiState {
  theme: ColorThemeId;
  visualStyle: VisualStyleId;
  accent: string;
  customTheme: CustomTheme;
  contrast: number;
  accentTint: AccentTint;
  uiFont: string;
  codeFont: string;
  fontSize: number | null;
  fontSmoothing: boolean;
  density: UiDensity;
  radiusScale: number;
  motionScale: number;
  streamReveal: "smooth" | "typewriter";
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  dockWidth: number;
  completionSound: boolean;
}
