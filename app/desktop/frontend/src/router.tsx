// TanStack Router tree, built dynamically from plugin-registered routes.
// AppRouter mounts after PluginProvider so the registry is already
// populated. Plugin routes are queried by id; they don't show up in
// the type-safe `<Link to="…">` autocomplete (the declare module below
// is keyed off the router shape, not the runtime route list).

import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { listRoutes } from "@/plugins/sdk";

const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

function buildRouter() {
  const specs = listRoutes();
  const routes = specs.map((spec) =>
    createRoute({
      getParentRoute: () => rootRoute,
      path: spec.path,
      // TanStack's RouteComponent expects an FC, not the broader
      // `ComponentType` (which includes class components). Plugins type
      // their `component` field as `ComponentType` so they can ship either;
      // cast here since TanStack will call it like a function in practice.
      component: spec.component as Parameters<typeof createRoute>[0]["component"],
    }),
  );
  return createRouter({
    routeTree: rootRoute.addChildren(routes),
    defaultPreload: "intent",
  });
}

// TanStack Router's type registration — used by <Link/> and useNavigate().
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}

export function AppRouter() {
  // The router is constructed once when AppRouter first renders. By then
  // PluginProvider has loaded built-in plugins synchronously and the
  // registry is populated. Sideloaded plugins that arrive later won't
  // appear until the next reload — pluginifying *that* requires a
  // `rebuildRouter()` host API, which we'll add only when there's a real
  // need (sideloaded routes are not on the current roadmap).
  const router = buildRouter();
  return <RouterProvider router={router} />;
}
