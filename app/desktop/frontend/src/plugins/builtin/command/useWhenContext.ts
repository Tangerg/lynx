// Context for `when` clauses. Exposes: mainViewActive, mainView (id),
// theme (id), scheme ("dark" | "light"), sidebarCollapsed. Unknown
// identifiers evaluate to undefined → falsy. Prefer `scheme` over
// `theme` in clauses so custom theme plugins still match.

import type { WhenContext } from "@/plugins/sdk";
import { useMemo } from "react";
import { resolveThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { useUiStore } from "@/state/uiStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";

export function useWhenContext(): WhenContext {
  const activeMainView = useWorkspaceSurfaceStore((s) => s.activeMainView);
  const theme = useUiStore((s) => s.theme);
  const sidebarCollapsed = useUiStore((s) => s.sidebarCollapsed);

  return useMemo(
    () => ({
      mainViewActive: !!activeMainView,
      mainView: activeMainView ?? "",
      theme,
      scheme: resolveThemeScheme(theme),
      sidebarCollapsed,
    }),
    [activeMainView, theme, sidebarCollapsed],
  );
}
