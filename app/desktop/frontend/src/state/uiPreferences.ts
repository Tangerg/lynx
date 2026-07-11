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
  radiusScale: number;
  motionScale: number;
  messageStyle: "bubble" | "plain";
  streamReveal: "smooth" | "typewriter";
  splitRatio: number;
  sidebarRail: boolean;
  dockCollapsed: boolean;
  completionSound: boolean;
}
