// Sidebar component types — kept small and independent of the AG-UI / mock
// layers so the sidebar can be reused with any session source.

export type SidebarSession = {
  id: string;
  title: string;
  status: "running" | "waiting" | "idle";
  model: string;
  time: string;
};

export type SidebarProject = {
  id: string;
  name: string;
  branch: string;
  active?: boolean;
};

/**
 * A theme id — references a `ThemeSpec` registered via
 * `host.theme.registerTheme()`. Built-ins ship as `"dark"` and `"light"`;
 * plugins can add more (`"solarized-dark"`, `"github-light"`, …).
 *
 * Code that needs the binary dark/light distinction (asset selection,
 * shiki/mermaid preset, etc.) should read the active theme's `scheme`
 * via `useActiveScheme()` instead of comparing the id directly.
 */
export type Theme = string;

/** Binary theme kind — the discriminator structural CSS keys on. */
export type ThemeScheme = "dark" | "light";
