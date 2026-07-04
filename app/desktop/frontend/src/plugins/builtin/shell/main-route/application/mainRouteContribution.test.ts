import { describe, expect, it } from "vitest";
import { mainRoute } from "./mainRouteContribution";

function Component() {
  return null;
}

describe("mainRoute", () => {
  it("projects the main page component into the root route spec", () => {
    expect(mainRoute(Component)).toEqual({
      id: "main",
      path: "/",
      component: Component,
      order: 0,
    });
  });
});
