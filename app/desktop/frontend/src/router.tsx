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

// A hand-typed or stale URL is input like any other: anything that isn't a
// non-empty string is not a location, it's noise, and becomes absent.
function param(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function locationOf(search: AppSearch): AppLocation {
  return {
    session: search.session ?? "",
    view: search.view ?? null,
    dock: search.dock ?? null,
    settings: search.settings ?? null,
  };
}

function searchOf(location: AppLocation): AppSearch {
  return {
    session: location.session || undefined,
    view: location.view ?? undefined,
    dock: location.dock ?? undefined,
    settings: location.settings ?? undefined,
  };
}

const rootRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>): AppSearch => ({
    session: param(search.session),
    view: param(search.view),
    dock: param(search.dock),
    settings: param(search.settings),
  }),
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
  });
}

// TanStack Router's type registration — used by <Link/> and useNavigate().
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}

function routerNavigator(router: ReturnType<typeof buildRouter>): Navigator {
  const read = (): AppLocation => locationOf(router.state.location.search);

  return {
    get: read,
    // Subscribed directly rather than through useRouterState, which selects
    // against the whole router state: this compares only the field the caller
    // asked for, so opening a settings pane doesn't re-render everything that
    // reads the dock. It also works outside RouterProvider, because the router
    // comes from this closure instead of from context — the ports built on this
    // are consumed from both sides of that boundary.
    use: (select) =>
      useSyncExternalStore(
        (onChange) => router.subscribe("onResolved", onChange),
        () => select(read()),
      ),
    subscribe(listener) {
      let previous = read();
      return router.subscribe("onResolved", () => {
        const next = read();
        if (sameLocation(previous, next)) return;
        const before = previous;
        previous = next;
        listener(next, before);
      });
    },
    go(patch, options) {
      const next = applyPatch(read(), patch);
      void router.navigate({
        to: "/",
        search: searchOf(next),
        replace: options?.replace === true,
      });
    },
    back: () => router.history.back(),
    forward: () => router.history.forward(),
  };
}

// Built once, on first access, together with the navigator that reads it. The
// router used to be rebuilt on every AppRouter render, which was invisible only
// because AppRouter renders once — a navigator holding a router that gets
// replaced underneath it would not have been.
let instance: ReturnType<typeof buildRouter> | null = null;

function appRouter(): ReturnType<typeof buildRouter> {
  if (!instance) {
    instance = buildRouter();
    configureNavigator(routerNavigator(instance));
  }
  return instance;
}

export function AppRouter() {
  // By the time this renders, PluginProvider has loaded built-in plugins
  // synchronously and the registry is populated. Sideloaded plugins that arrive
  // later won't appear until the next reload — pluginifying *that* requires a
  // `rebuildRouter()` host API, which we'll add only when there's a real need
  // (sideloaded routes are not on the current roadmap).
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
