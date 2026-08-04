// Dev-only inspector for the two TanStack subsystems the app leans on hardest:
// the query cache — invalidated from dozens of places, with no way to watch one
// land — and the router.
//
// Gated twice on purpose. `import.meta.env.DEV` folds to `false` in a build, so
// the ternary below is dead code and the dynamic import is never emitted: the
// panels cannot reach production even by accident, which is why the three
// packages behind them are devDependencies. check-bundle-size is the proof —
// the startup payload does not move.

import { lazy, Suspense } from "react";
import type { AnyRouter } from "@tanstack/react-router";

const Panels = import.meta.env.DEV ? lazy(() => import("./devtoolsPanels")) : null;

export function Devtools({ router }: { router: AnyRouter }) {
  if (!Panels) return null;
  return (
    <Suspense>
      <Panels router={router} />
    </Suspense>
  );
}
