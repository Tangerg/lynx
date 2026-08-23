import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useActionMenu } from "./useActionMenu";

let callbacks: FrameRequestCallback[] = [];

beforeEach(() => {
  callbacks = [];
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    callbacks.push(callback);
    return callbacks.length;
  });
  vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => undefined);
});

afterEach(() => vi.restoreAllMocks());

describe("useActionMenu", () => {
  it("owns focus traversal, IME-safe escape, and trigger restoration", () => {
    render(<MenuProbe />);
    const trigger = screen.getByRole("button", { name: "menu" });
    fireEvent.click(trigger);
    flushFrames();
    const first = screen.getByRole("menuitem", { name: "first" });
    const last = screen.getByRole("menuitem", { name: "last" });
    expect(document.activeElement).toBe(first);

    fireEvent.keyDown(first, { key: "ArrowUp" });
    expect(document.activeElement).toBe(last);
    fireEvent.keyDown(last, { key: "Home" });
    expect(document.activeElement).toBe(first);

    fireEvent.keyDown(first, { key: "Escape", isComposing: true });
    expect(screen.queryByRole("menu")).not.toBeNull();
    fireEvent.keyDown(first, { key: "Escape" });
    flushFrames();
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});

function MenuProbe() {
  const menu = useActionMenu<
    HTMLDivElement,
    HTMLButtonElement,
    HTMLDivElement
  >();
  return (
    <div ref={menu.rootRef}>
      <button ref={menu.triggerRef} type="button" onClick={menu.toggle}>
        menu
      </button>
      {menu.open ? (
        <div ref={menu.menuRef} role="menu">
          <button type="button" role="menuitem">
            first
          </button>
          <button type="button" role="menuitem" disabled>
            disabled
          </button>
          <button type="button" role="menuitem">
            last
          </button>
        </div>
      ) : null}
    </div>
  );
}

function flushFrames() {
  act(() => {
    const pending = callbacks;
    callbacks = [];
    for (const callback of pending) callback(performance.now());
  });
}
