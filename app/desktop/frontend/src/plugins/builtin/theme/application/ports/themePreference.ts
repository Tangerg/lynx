import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * The stored theme choice, as this context needs it: read the current id, write
 * the next one. Reached through a port rather than the UI store directly —
 * deciding which theme comes next is this context's rule, and a rule that
 * imports a global store cannot be exercised without one.
 */
export interface ThemePreferencePort {
  activeTheme(): string;
  setTheme(id: string): void;
}

const port = createSingletonPort<ThemePreferencePort>("Theme preference port is not configured");

export const configureThemePreferencePort = port.configure;
export const themePreference = port.get;
