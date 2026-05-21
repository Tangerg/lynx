// Build the context object that `when` clauses evaluate against.
//
// Adds keys for shell-level UI state that commands typically gate on:
//   - mainViewActive: a workspace view is open in the main area
//   - mainView:       its id (e.g. "settings", "diff"), "" otherwise
//   - theme:          "dark" | "light"
//   - sidebarRail:    sidebar is in collapsed-rail mode
//
// New keys can be added incrementally — the evaluator treats unknown
// identifiers as `undefined`, which negates / compares as falsy.

import { useMemo } from "react";
import type { WhenContext } from "@/plugins/sdk";
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
      sidebarRail,
    }),
    [activeMainView, theme, sidebarRail],
  );
}
