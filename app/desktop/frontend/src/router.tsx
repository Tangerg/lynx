// TanStack Router tree, built dynamically from plugin-registered routes, and
// the app's only Navigator implementation.
//
// AppRouter mounts after PluginProvider so the registry is already populated.
// Plugin routes are queried by id; they don't show up in the type-safe
// `<Link to="…">` autocomplete (the declare module below is keyed off the router
// shape, not the runtime route list).
//
// The search params are where the user's location lives — see lib/navigation for
// the ownership rule. They are declared on the root route so every plugin route
// inherits them: "which session / view / dock / settings pane" is a property of
// the app, not of one page.

import { useSyncExternalStore } from "react";
import {
  createBrowserHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { lookupExtensionPoint, ROUTE } from "@/plugins/sdk";
import {
  applyPatch,
  configureNavigator,
  sameLocation,
  type AppLocation,
  type Navigator,
} from "@/lib/navigation";
import { Devtools } from "@/Devtools";

/** The location, as it appears in the URL. Absent field = absent param. */
interface AppSearch {
  session?: string;
  view?: string;
  dock?: string;
  settings?: string;
}

// ONE parser, used by both readers below — the route's `validateSearch` and the
// Navigator were each free to read the URL their own way, which is two answers
// to "where am I" in the same file. A hand-typed or stale URL is input like any
// other: anything that isn't a non-empty string is not a location, it's noise.
function locationFrom(read: (key: string) => unknown): AppLocation {
  const param = (key: string): string | null => {
    const value = read(key);
    return typeof value === "string" && value.length > 0 ? value : null;
  };
  return {
    session: param("session") ?? "",
    view: param("view"),
    dock: param("dock"),
    settings: param("settings"),
  };
}

// The location lives in the URL, and the URL is owned by ONE history instance —
// created here, handed to the router below, and read by the Navigator. It exists
// before any plugin loads, which is what the Navigator needs: ports are
// installed while plugins load and several of them read or subscribe to the
// location immediately, whereas the router cannot be built until those same
// plugins have registered their routes. The Navigator therefore binds directly
// to history rather than depending on the later router construction boundary.
const history = createBrowserHistory();

function readLocation(): AppLocation {
  const params = new URLSearchParams(history.location.search);
  return locationFrom((key) => params.get(key));
}

function searchOf(location: AppLocation): AppSearch {
  return {
    session: location.session || undefined,
    view: location.view ?? undefined,
    dock: location.dock ?? undefined,
    settings: location.settings ?? undefined,
  };
}

function hrefOf(location: AppLocation): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(searchOf(location))) {
    if (value !== undefined) params.set(key, value);
  }
  const query = params.toString();
  return query ? `${history.location.pathname}?${query}` : history.location.pathname;
}

const historyNavigator: Navigator = {
  get: readLocation,
  // Compares only the field the caller asked for, so opening a settings pane
  // doesn't re-render everything that reads the dock.
  use: (select) =>
    useSyncExternalStore(
      (onChange) => history.subscribe(onChange),
      () => select(readLocation()),
    ),
  subscribe(listener) {
    let previous = readLocation();
    return history.subscribe(() => {
      const next = readLocation();
      if (sameLocation(previous, next)) return;
      const before = previous;
      previous = next;
      listener(next, before);
    });
  },
  go(patch, options) {
    const current = readLocation();
    const next = applyPatch(current, patch);
    if (sameLocation(current, next)) return;
    const href = hrefOf(next);
    if (options?.replace === true) history.replace(href);
    else history.push(href);
  },
  back: () => history.back(),
  forward: () => history.forward(),
};

configureNavigator(historyNavigator);

const rootRoute = createRootRoute({
  // Declared here so every plugin route inherits it, and derived from the same
  // parser the Navigator reads with, so the route can never disagree about what
  // the URL says.
  validateSearch: (search: Record<string, unknown>): AppSearch =>
    searchOf(locationFrom((key) => search[key])),
  component: () => <Outlet />,
});

function buildRouter() {
  const specs = lookupExtensionPoint(ROUTE);
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
    history,
  });
}

// TanStack Router's type registration — used by <Link/> and useNavigate().
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}

// Built once on first access: route identity must remain stable across AppRouter
// renders after the plugin registry has supplied the initial route set.
let instance: ReturnType<typeof buildRouter> | null = null;

function appRouter(): ReturnType<typeof buildRouter> {
  return (instance ??= buildRouter());
}

export function AppRouter() {
  // By the time this renders, PluginProvider has loaded every route-contributing
  // built-in and the registry is complete, so route identity can stay stable.
  const router = appRouter();
  return (
    <>
      <RouterProvider router={router} />
      {/* Beside the provider rather than inside it: RouterProvider renders the
          matched route tree, so it has no slot to put this in. The router goes
          down as a prop instead. Compiles to nothing in a build. */}
      <Devtools router={router} />
    </>
  );
}
