import { describe, expect, it } from "vitest";
import {
  canPresentDock,
  clampDockWidth,
  clampSidebarWidth,
  DOCK_MIN_WIDTH_PX,
  maxDockWidth,
  maxSidebarWidth,
  SIDEBAR_MIN_WIDTH_PX,
} from "./shellGeometry";

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

describe("dock geometry", () => {
  it("preserves the conversation floor and an even split while the row has room", () => {
    expect(maxDockWidth(1120)).toBe(480);
    expect(clampDockWidth(720, 1120)).toBe(480);
    expect(clampDockWidth(420, 1120)).toBe(420);
  });

  it("folds the dock when both columns cannot keep their operable floors", () => {
    expect(canPresentDock(1059)).toBe(false);
    expect(canPresentDock(1060)).toBe(true);
    expect(maxDockWidth(640)).toBe(DOCK_MIN_WIDTH_PX);
    expect(clampDockWidth(100, 640)).toBe(DOCK_MIN_WIDTH_PX);
  });
});
