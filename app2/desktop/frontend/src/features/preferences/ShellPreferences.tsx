import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import "./theme.css";
import {
  detectLocale,
  isLocale,
  type Locale,
} from "../localization/locales";

export const themes = [
  { id: "system" },
  { id: "linen" },
  { id: "graphite" },
] as const;

export const accents = [
  { id: "ember" },
  { id: "ocean" },
  { id: "forest" },
  { id: "violet" },
] as const;

export type ThemePreference = (typeof themes)[number]["id"];
export type AccentPreference = (typeof accents)[number]["id"];

interface ShellPreferenceState {
  theme: ThemePreference;
  accent: AccentPreference;
  locale: Locale;
}

interface ShellPreferenceContext extends ShellPreferenceState {
  resolvedTheme: "linen" | "graphite";
  setTheme(theme: ThemePreference): void;
  setAccent(accent: AccentPreference): void;
  setLocale(locale: Locale): void;
}

const storageKey = "lyra.app2.shell.v2";
const ShellPreferencesContext = createContext<ShellPreferenceContext | undefined>(
  undefined,
);

export function ShellPreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(readPreferences);
  const [systemDark, setSystemDark] = useState(systemPrefersDark);
  const resolvedTheme =
    preferences.theme === "system"
      ? systemDark
        ? "graphite"
        : "linen"
      : preferences.theme;

  useLayoutEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => setSystemDark(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useLayoutEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = resolvedTheme;
    root.dataset.accent = preferences.accent;
    root.style.colorScheme = resolvedTheme === "graphite" ? "dark" : "light";
    document
      .querySelector<HTMLMetaElement>('meta[name="theme-color"]')
      ?.setAttribute(
        "content",
        resolvedTheme === "graphite" ? "#171816" : "#f2f0eb",
      );
  }, [preferences.accent, resolvedTheme]);

  const setTheme = useCallback((theme: ThemePreference) => {
    setPreferences((current) => {
      const next = { ...current, theme };
      writePreferences(next);
      return next;
    });
  }, []);
  const setAccent = useCallback((accent: AccentPreference) => {
    setPreferences((current) => {
      const next = { ...current, accent };
      writePreferences(next);
      return next;
    });
  }, []);
  const setLocale = useCallback((locale: Locale) => {
    setPreferences((current) => {
      const next = { ...current, locale };
      writePreferences(next);
      return next;
    });
  }, []);
  const value = useMemo<ShellPreferenceContext>(
    () => ({
      ...preferences,
      resolvedTheme,
      setTheme,
      setAccent,
      setLocale,
    }),
    [preferences, resolvedTheme, setAccent, setLocale, setTheme],
  );

  return (
    <ShellPreferencesContext.Provider value={value}>
      {children}
    </ShellPreferencesContext.Provider>
  );
}

export function useShellPreferences() {
  const preferences = useContext(ShellPreferencesContext);
  if (preferences === undefined) {
    throw new Error("Shell preferences require ShellPreferencesProvider");
  }
  return preferences;
}

function systemPrefersDark() {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function readPreferences(): ShellPreferenceState {
  const defaults: ShellPreferenceState = {
    theme: "system",
    accent: "ember",
    locale: detectLocale(navigator.languages),
  };
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (raw === null) return defaults;
    const value: unknown = JSON.parse(raw);
    if (
      !isRecord(value) ||
      Object.keys(value).length !== 3 ||
      !themes.some((theme) => theme.id === value.theme) ||
      !accents.some((accent) => accent.id === value.accent) ||
      !isLocale(value.locale)
    ) {
      return defaults;
    }
    return {
      theme: value.theme as ThemePreference,
      accent: value.accent as AccentPreference,
      locale: value.locale,
    };
  } catch {
    return defaults;
  }
}

function writePreferences(preferences: ShellPreferenceState) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(preferences));
  } catch {
    // The active in-memory preference remains valid when persistence is unavailable.
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
