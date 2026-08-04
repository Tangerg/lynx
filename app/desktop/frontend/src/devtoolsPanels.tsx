// The dev inspector's actual panels. Reached only through the dynamic import in
// Devtools.tsx, which is what keeps these three dev packages out of the
// shipped bundle.
//
// One shell hosting both panels rather than two floating widgets: they would
// otherwise stack their own triggers in the same corner of a window that is
// already dense.

import type { AnyRouter } from "@tanstack/react-router";
import { TanStackDevtools } from "@tanstack/react-devtools";
import { ReactQueryDevtoolsPanel } from "@tanstack/react-query-devtools";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";

export default function DevtoolsPanels({ router }: { router: AnyRouter }) {
  return (
    <TanStackDevtools
      plugins={[
        {
          id: "query",
          name: "Query",
          render: <ReactQueryDevtoolsPanel />,
        },
        {
          id: "router",
          name: "Router",
          // Passed explicitly: this renders beside RouterProvider, not under
          // it, so there is no router context here to fall back on.
          render: <TanStackRouterDevtoolsPanel router={router} />,
        },
      ]}
    />
  );
}
