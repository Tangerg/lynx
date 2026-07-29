import type { UiDensity } from "@/lib/density";
import type { DockDensity } from "@/lib/shellGeometry";

/** A registered theme id. `system` resolves against the current OS scheme. */
export type Theme = string;

export interface CustomTheme {
  bg: string;
  fg: string;
}

export interface UiState {
  theme: Theme;
  accent: string;
  customTheme: CustomTheme;
  contrast: number;
  uiFont: string;
  codeFont: string;
  fontSize: number | null;
  fontSmoothing: boolean;
  density: UiDensity;
  radiusScale: number;
  motionScale: number;
  messageStyle: "bubble" | "plain";
  streamReveal: "smooth" | "typewriter";
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  dockWidths: Record<DockDensity, number>;
  completionSound: boolean;
}
