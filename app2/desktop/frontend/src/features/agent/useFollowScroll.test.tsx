import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useFollowScroll } from "./useFollowScroll";

let nextFrame = 1;
let frames = new Map<number, FrameRequestCallback>();

beforeEach(() => {
  nextFrame = 1;
  frames = new Map();
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    const id = nextFrame++;
    frames.set(id, callback);
    return id;
  });
  vi.spyOn(window, "cancelAnimationFrame").mockImplementation((id) => {
    frames.delete(id);
  });
});

afterEach(() => vi.restoreAllMocks());

describe("useFollowScroll", () => {
  it("escapes on reader scroll and explicitly follows new material", () => {
    const view = render(<Probe version="1" />);
    const viewport = screen.getByTestId("viewport");
    let scrollTop = 900;
    Object.defineProperties(viewport, {
      scrollHeight: { configurable: true, get: () => 1000 },
      clientHeight: { configurable: true, get: () => 100 },
      scrollTop: {
        configurable: true,
        get: () => scrollTop,
        set: (value: number) => {
          scrollTop = value;
        },
      },
      scrollTo: {
        configurable: true,
        value: ({ top }: ScrollToOptions) => {
          scrollTop = Number(top);
        },
      },
    });
    flushFrames();
    expect(screen.getByTestId("state").textContent).toBe("following:false");

    scrollTop = 100;
    fireEvent.scroll(viewport);
    expect(screen.getByTestId("state").textContent).toBe("reading:false");

    view.rerender(<Probe version="2" />);
    expect(screen.getByTestId("state").textContent).toBe("reading:true");
    fireEvent.click(screen.getByRole("button", { name: "latest" }));
    flushFrames();
    expect(screen.getByTestId("state").textContent).toBe("following:false");
    expect(scrollTop).toBe(1000);
  });
});

function Probe({ version }: { version: string }) {
  const follow = useFollowScroll(version, 24);
  return (
    <>
      <div data-testid="state">
        {follow.following ? "following" : "reading"}:
        {String(follow.hasNewMaterial)}
      </div>
      <div
        ref={follow.viewportRef}
        data-testid="viewport"
        onScroll={follow.onScroll}
      >
        <div ref={follow.contentRef}>{version}</div>
      </div>
      <button type="button" onClick={follow.follow}>
        latest
      </button>
    </>
  );
}

function flushFrames() {
  act(() => {
    while (frames.size > 0) {
      const pending = [...frames.entries()];
      frames.clear();
      for (const [, callback] of pending) callback(performance.now());
    }
  });
}
