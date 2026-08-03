import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ResizeHandle } from "./resize-handle";

const PROPERTY = "--pane-width";

function Harness({
  edge = "end",
  value = 320,
  onCommit,
}: {
  edge?: "start" | "end";
  value?: number;
  onCommit: (width: number) => void;
}) {
  return (
    <div data-testid="container">
      <ResizeHandle
        aria-label="Resize panel"
        edge={edge}
        value={value}
        container={(handle) => handle.parentElement}
        property={PROPERTY}
        read={(container) => Number.parseFloat(container.style.getPropertyValue(PROPERTY)) || value}
        minWidth={240}
        maxWidth={() => 800}
        onCommit={onCommit}
        resizingAttribute="data-resizing"
      />
    </div>
  );
}

function paneWidth(): string {
  return screen.getByTestId("container").style.getPropertyValue(PROPERTY);
}

describe("ResizeHandle", () => {
  it("owns accessible separator semantics", () => {
    render(<Harness onCommit={() => {}} />);

    const handle = screen.getByRole("separator", { name: "Resize panel" });
    expect(handle.getAttribute("aria-orientation")).toBe("vertical");
    expect(handle.getAttribute("tabindex")).toBe("0");
    expect(handle.getAttribute("aria-valuemin")).toBe("240");
    expect(handle.getAttribute("aria-valuenow")).toBe("320");
    expect(handle.getAttribute("aria-valuemax")).toBe("800");
  });

  it("writes the width straight onto the container during a drag and commits on release", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.pointerDown(handle, { button: 0, clientX: 320 });
    expect(screen.getByTestId("container").hasAttribute("data-resizing")).toBe(true);

    fireEvent(window, new MouseEvent("pointermove", { clientX: 400 }));
    expect(paneWidth()).toBe("400px");
    expect(handle.getAttribute("aria-valuenow")).toBe("400");
    expect(onCommit).not.toHaveBeenCalled();

    fireEvent(window, new MouseEvent("pointerup"));
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(400);
    expect(screen.getByTestId("container").hasAttribute("data-resizing")).toBe(false);
  });

  it("does not commit a press that never moved", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.pointerDown(handle, { button: 0, clientX: 320 });
    fireEvent(window, new MouseEvent("pointerup"));

    expect(onCommit).not.toHaveBeenCalled();
    expect(paneWidth()).toBe("");
  });

  it("grows toward the pane, whichever edge it sits on", () => {
    const onCommit = vi.fn();
    const { unmount } = render(<Harness edge="end" onCommit={onCommit} />);
    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize panel" }), {
      key: "ArrowRight",
    });
    expect(onCommit).toHaveBeenLastCalledWith(328);
    unmount();

    render(<Harness edge="start" onCommit={onCommit} />);
    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize panel" }), {
      key: "ArrowRight",
    });
    expect(onCommit).toHaveBeenLastCalledWith(312);
  });

  it("takes a coarse step with Shift and jumps to the range ends", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
    expect(onCommit).toHaveBeenLastCalledWith(344);
    fireEvent.keyDown(handle, { key: "Home" });
    expect(onCommit).toHaveBeenLastCalledWith(240);
    fireEvent.keyDown(handle, { key: "End" });
    expect(onCommit).toHaveBeenLastCalledWith(800);
  });

  // Key repeat holds the mark, so the pane must not animate between steps; releasing the
  // key — or losing focus while it is still down — has to hand the animation back.
  it("holds the resizing mark across key repeats and releases it on blur", () => {
    render(<Harness onCommit={() => {}} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });
    const container = screen.getByTestId("container");

    fireEvent.keyDown(handle, { key: "ArrowRight" });
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(container.hasAttribute("data-resizing")).toBe(true);

    fireEvent.blur(handle);
    expect(container.hasAttribute("data-resizing")).toBe(false);
  });

  it("leaves nothing attached when unmounted mid-drag", () => {
    const onCommit = vi.fn();
    const { unmount } = render(<Harness onCommit={onCommit} />);
    const container = screen.getByTestId("container");

    fireEvent.pointerDown(screen.getByRole("separator", { name: "Resize panel" }), {
      button: 0,
      clientX: 320,
    });
    unmount();

    expect(container.hasAttribute("data-resizing")).toBe(false);
    fireEvent(window, new MouseEvent("pointermove", { clientX: 500 }));
    fireEvent(window, new MouseEvent("pointerup"));
    expect(onCommit).not.toHaveBeenCalled();
  });
});
