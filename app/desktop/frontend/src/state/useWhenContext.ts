// Build the context object that `when` clauses evaluate against.
//
// Adds keys for shell-level UI state that commands typically gate on:
//   - mainViewActive: a workspace view is open in the main area
//   - mainView:       its id (e.g. "settings", "diff"), "" otherwise
//   - theme:          active theme id ("dark", "light", or any custom)
//   - scheme:         binary kind — "dark" | "light"
//   - sidebarRail:    sidebar is in collapsed-rail mode
//
// New keys can be added incrementally — the evaluator treats unknown
// identifiers as `undefined`, which negates / compares as falsy.
//
// `when` clauses checking the binary kind ("scheme == 'light'") should
// prefer `scheme` over `theme` so custom theme plugins work.

import { useMemo } from "react";
import { resolveScheme, type WhenContext } from "@/plugins/sdk";
import { useUIStore } from "./uiStore";

export function useWhenContext(): WhenContext {
  const activeMainView = useUIStore((s) => s.activeMainView);
  const theme = useUIStore((s) => s.theme);
  const sidebarRail = useUIStore((s) => s.sidebarRail);

  return useMemo(
    () => ({
      mainViewActive: !!activeMainView,
      mainView: activeMainView ?? "",
      theme,
      scheme: resolveScheme(theme),
      sidebarRail,
    }),
    [activeMainView, theme, sidebarRail],
  );
}
