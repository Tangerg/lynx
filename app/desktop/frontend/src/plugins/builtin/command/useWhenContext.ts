// Context for `when` clauses. Exposes: mainViewActive, mainView (id),
// theme (id), scheme ("dark" | "light"), sidebarCollapsed, paletteOpen. Unknown
// identifiers evaluate to undefined → falsy. Prefer `scheme` over
// `theme` in clauses so custom theme plugins still match.

import type { WhenContext } from "@/plugins/sdk";
import { useMemo } from "react";
import { resolveThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { usePaletteStore } from "./paletteStore";
import { useUiStore } from "@/state/uiStore";
import { navigator } from "@/lib/navigation";

export function useWhenContext(): WhenContext {
  const activeMainView = navigator().use((location) => location.view);
  const theme = useUiStore((s) => s.theme);
  const sidebarCollapsed = useUiStore((s) => s.sidebarCollapsed);
  const paletteOpen = usePaletteStore((s) => s.open);

  return useMemo(
    () => ({
      mainViewActive: !!activeMainView,
      mainView: activeMainView ?? "",
      theme,
      scheme: resolveThemeScheme(theme),
      sidebarCollapsed,
      paletteOpen,
    }),
    [activeMainView, theme, sidebarCollapsed, paletteOpen],
  );
}
