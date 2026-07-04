import type { RouteSpec } from "@/plugins/sdk";

export function mainRoute(component: RouteSpec["component"]): RouteSpec {
  return {
    id: "main",
    path: "/",
    component,
    order: 0,
  };
}
