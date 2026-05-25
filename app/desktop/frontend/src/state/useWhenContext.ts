// Context for `when` clauses. Exposes: mainViewActive, mainView (id),
// theme (id), scheme ("dark" | "light"), sidebarRail. Unknown
// identifiers evaluate to undefined → falsy. Prefer `scheme` over
// `theme` in clauses so custom theme plugins still match.

import type {WhenContext} from "@/plugins/sdk";
import { useMemo } from "react";
import { resolveScheme  } from "@/plugins/sdk";
import { useLayoutStore } from "./layoutStore";
import { useSessionStore } from "./sessionStore";
import { useThemeStore } from "./themeStore";

export function useWhenContext(): WhenContext {
  const activeMainView = useSessionStore((s) => s.activeMainView);
  const theme = useThemeStore((s) => s.theme);
  const sidebarRail = useLayoutStore((s) => s.sidebarRail);

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
