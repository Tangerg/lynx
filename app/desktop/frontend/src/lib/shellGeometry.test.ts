import { describe, expect, it } from "vitest";
import { clampSidebarWidth, maxSidebarWidth, SIDEBAR_MIN_WIDTH_PX } from "./shellGeometry";

describe("sidebar geometry", () => {
  it("preserves the reading column while the window has room", () => {
    expect(maxSidebarWidth(1440)).toBe(800);
    expect(clampSidebarWidth(900, 1440)).toBe(800);
    expect(clampSidebarWidth(320, 1440)).toBe(320);
  });

  it("keeps the drawer operable in a window narrower than both columns", () => {
    expect(maxSidebarWidth(720)).toBe(SIDEBAR_MIN_WIDTH_PX);
    expect(clampSidebarWidth(100, 720)).toBe(SIDEBAR_MIN_WIDTH_PX);
  });
});
