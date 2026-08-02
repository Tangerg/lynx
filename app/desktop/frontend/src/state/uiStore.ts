// Persisted UI preferences — colour theme + visual style + accent + fonts +
// sidebar collapse state. Single Zustand store + single persistence key
// since every field is "what the user's UI should look like across
// launches". The side-effects at the bottom of this file mirror the
// active appearance specs + font preferences to :root (inline CSS vars +
// theme-{scheme} class on <html>).

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { DEFAULT_UI_DENSITY, UI_DENSITY_MODES, type UiDensity } from "@/lib/density";
import type { ColorThemeId, VisualStyleId } from "@/lib/appearance";
import { DOCK_DEFAULT_WIDTH_PX, SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
// Direct registry import — going through the SDK barrel pulls in
// host.ts which imports this file, creating a TDZ cycle under Vitest.
// Same reason the extension-point reads below import from the deep
// `selectors/extensions` + `kernelPoints` paths (neither pulls host).
import type { CustomTheme, UiState } from "./uiPreferences";

export type { CustomTheme, UiState } from "./uiPreferences";

// localStorage payload schema. Validated on rehydrate so a corrupted
// `lyra.ui` entry (manual edit, downgrade leaving a future-shape blob,
// browser extension tampering) falls back to defaults instead of
// crashing the boot.
const uiPersistSchema = z.object({
  theme: z.string(),
  visualStyle: z.string(),
  accent: z.string(),
  customTheme: z.object({ bg: z.string(), fg: z.string() }),
  contrast: z.number(),
  uiFont: z.string(),
  codeFont: z.string(),
  fontSize: z.number().nullable(),
  fontSmoothing: z.boolean(),
  density: z.enum(UI_DENSITY_MODES),
  radiusScale: z.number(),
  motionScale: z.number(),
  streamReveal: z.enum(["smooth", "typewriter"]),
  sidebarCollapsed: z.boolean(),
  sidebarWidth: z.number(),
  dockWidth: z.number(),
  completionSound: z.boolean(),
});

interface UiActions {
  setTheme: (theme: ColorThemeId) => void;
  setVisualStyle: (visualStyle: VisualStyleId) => void;
  /**
   * Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
   * theme plugins still toggle sensibly. Picks the first registered
   * theme whose scheme is the opposite of the current one; no-op if
   * none exists (e.g. only dark themes registered).
   */
  setAccent: (accent: string) => void;
  /** Patch one or more of the custom theme's base colors. */
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
  setContrast: (contrast: number) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setDensity: (density: UiDensity) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
  setStreamReveal: (mode: "smooth" | "typewriter") => void;
  toggleSidebar: () => void;
  setSidebarWidth: (width: number) => void;
  setDockWidth: (width: number) => void;
  setCompletionSound: (on: boolean) => void;
}

export const useUiStore = create<UiState & UiActions>()(
  persist(
    (set) => ({
      theme: "light",
      visualStyle: "lyra",
      accent: "#006bff",
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
      contrast: 25,
      uiFont: "",
      codeFont: "",
      fontSize: null,
      fontSmoothing: true,
      density: DEFAULT_UI_DENSITY,
      radiusScale: 1,
      motionScale: 1,
      streamReveal: "smooth",
      sidebarCollapsed: false,
      sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
      dockWidth: DOCK_DEFAULT_WIDTH_PX,
      completionSound: false,

      setTheme: (theme) => set({ theme }),
      setVisualStyle: (visualStyle) => set({ visualStyle }),
      setAccent: (accent) => set({ accent }),
      setCustomTheme: (patch) => set((s) => ({ customTheme: { ...s.customTheme, ...patch } })),
      setContrast: (contrast) => set({ contrast }),
      setUiFont: (uiFont) => set({ uiFont }),
      setCodeFont: (codeFont) => set({ codeFont }),
      setFontSize: (fontSize) => set({ fontSize }),
      setFontSmoothing: (fontSmoothing) => set({ fontSmoothing }),
      setDensity: (density) => set({ density }),
      setRadiusScale: (radiusScale) => set({ radiusScale }),
      setMotionScale: (motionScale) => set({ motionScale }),
      setStreamReveal: (streamReveal) => set({ streamReveal }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarWidth: (sidebarWidth) => set({ sidebarWidth }),
      setDockWidth: (dockWidth) => set({ dockWidth }),
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: "lyra.ui",
      storage: createJSONStorage(() => localStorage),
      version: 10,
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = uiPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          // Reset on schema mismatch — defaults are always a safe boot.
          console.warn("[uiStore] discarding corrupted lyra.ui:", parsed.error.issues);
          return current;
        }
        return { ...current, ...parsed.data };
      },
    },
  ),
);
