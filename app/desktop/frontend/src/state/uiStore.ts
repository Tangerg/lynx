// Persisted UI preferences — theme + accent + fonts + message-style +
// sidebar collapse state. Single Zustand store + single persistence key
// since every field is "what the user's UI should look like across
// launches". The side-effects at the bottom of this file mirror the
// active theme spec + font preferences to :root (inline CSS vars +
// theme-{scheme} class on <html>).

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
// Direct registry import — going through the SDK barrel pulls in
// host.ts which imports this file, creating a TDZ cycle under Vitest.
// Same reason the extension-point reads below import from the deep
// `selectors/extensions` + `kernelPoints` paths (neither pulls host).
import { THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import type { CustomTheme, Theme, UiState } from "./uiPreferences";
import { installUiStoreEffects, resolveThemeId } from "./uiStoreEffects";

export type { CustomTheme, Theme, UiState } from "./uiPreferences";

// localStorage payload schema. Validated on rehydrate so a corrupted
// `lyra.ui` entry (manual edit, downgrade leaving a future-shape blob,
// browser extension tampering) falls back to defaults instead of
// crashing the boot.
const uiPersistSchema = z.object({
  theme: z.string(),
  accent: z.string(),
  customTheme: z.object({ bg: z.string(), fg: z.string() }),
  contrast: z.number(),
  uiFont: z.string(),
  codeFont: z.string(),
  fontSize: z.number().nullable(),
  fontSmoothing: z.boolean(),
  radiusScale: z.number(),
  motionScale: z.number(),
  messageStyle: z.enum(["bubble", "plain"]),
  streamReveal: z.enum(["smooth", "typewriter"]),
  splitRatio: z.number(),
  sidebarRail: z.boolean(),
  dockCollapsed: z.boolean(),
  completionSound: z.boolean(),
});

interface UiActions {
  setTheme: (theme: Theme) => void;
  /**
   * Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
   * theme plugins still toggle sensibly. Picks the first registered
   * theme whose scheme is the opposite of the current one; no-op if
   * none exists (e.g. only dark themes registered).
   */
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
  /** Patch one or more of the custom theme's base colors. */
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
  setContrast: (contrast: number) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
  setMessageStyle: (style: "bubble" | "plain") => void;
  setStreamReveal: (mode: "smooth" | "typewriter") => void;
  setSplitRatio: (ratio: number) => void;
  toggleSidebar: () => void;
  toggleDock: () => void;
  setCompletionSound: (on: boolean) => void;
}

export const useUiStore = create<UiState & UiActions>()(
  persist(
    (set, get) => ({
      theme: "light",
      accent: "#006bff",
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
      contrast: 60,
      uiFont: "",
      codeFont: "",
      fontSize: null,
      fontSmoothing: true,
      radiusScale: 1,
      motionScale: 1,
      messageStyle: "bubble",
      streamReveal: "smooth",
      splitRatio: 0.5,
      sidebarRail: false,
      dockCollapsed: false,
      completionSound: false,

      setTheme: (theme) => set({ theme }),
      toggleTheme: () => {
        const cur = resolveThemeId(get().theme);
        const curSpec = lookupExtensionByKey(THEME, cur);
        const curScheme = curSpec?.scheme ?? (cur === "light" ? "light" : "dark");
        const target = curScheme === "dark" ? "light" : "dark";
        // `lookupExtensionPoint` returns themes already sorted by `order`, so
        // the first match is the "primary" theme of the opposite scheme —
        // matches the sort the appearance pane uses.
        const candidates = lookupExtensionPoint(THEME).filter((t) => t.scheme === target);
        if (candidates[0]) set({ theme: candidates[0].id });
      },
      setAccent: (accent) => set({ accent }),
      setCustomTheme: (patch) => set((s) => ({ customTheme: { ...s.customTheme, ...patch } })),
      setContrast: (contrast) => set({ contrast }),
      setUiFont: (uiFont) => set({ uiFont }),
      setCodeFont: (codeFont) => set({ codeFont }),
      setFontSize: (fontSize) => set({ fontSize }),
      setFontSmoothing: (fontSmoothing) => set({ fontSmoothing }),
      setRadiusScale: (radiusScale) => set({ radiusScale }),
      setMotionScale: (motionScale) => set({ motionScale }),
      setMessageStyle: (messageStyle) => set({ messageStyle }),
      setStreamReveal: (streamReveal) => set({ streamReveal }),
      setSplitRatio: (splitRatio) => set({ splitRatio }),
      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),
      toggleDock: () => set((s) => ({ dockCollapsed: !s.dockCollapsed })),
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: "lyra.ui",
      storage: createJSONStorage(() => localStorage),
      version: 5,
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

disposeOnHmr(installUiStoreEffects(useUiStore));
