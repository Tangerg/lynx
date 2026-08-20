import { describe, expect, it } from "vitest";
import {
  canPresentDock,
  clampDockWidth,
  clampSidebarWidth,
  DOCK_MIN_WIDTH_PX,
  maxDockWidth,
  maxSidebarWidth,
  SIDEBAR_DEFAULT_WIDTH_PX,
  SIDEBAR_MIN_WIDTH_PX,
} from "./shellGeometry";

describe("sidebar geometry", () => {
  it("matches the Codex desktop default and bounded resize range", () => {
    expect(SIDEBAR_DEFAULT_WIDTH_PX).toBe(275);
    expect(maxSidebarWidth(1440)).toBe(520);
    expect(clampSidebarWidth(900, 1440)).toBe(520);
    expect(clampSidebarWidth(320, 1440)).toBe(320);
  });

  it("keeps both the drawer and reading plane operable in a narrow window", () => {
    expect(maxSidebarWidth(720)).toBe(480);
    expect(clampSidebarWidth(900, 720)).toBe(480);
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
