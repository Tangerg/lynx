import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { Slot } from "./Slot";
import { definePlugin, loadPlugin } from "./sdk";

describe("Slot", () => {
  it("renders nothing when no plugin has filled the slot", () => {
    const { container } = render(<Slot name="empty.slot" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders registered components ordered by `order`", async () => {
    await loadPlugin(
      definePlugin({
        name: "test.layout.a",
        version: "1.0.0",
        setup: ({ host }) => {
          host.layout.register("test.slot", { id: "a", order: 2, component: () => <span>A</span> });
          host.layout.register("test.slot", { id: "b", order: 1, component: () => <span>B</span> });
        },
      }),
    );
    const { container } = render(<Slot name="test.slot" />);
    // Order=1 (B) comes before order=2 (A) regardless of registration sequence.
    expect(container.textContent).toBe("BA");
  });

  it("wraps each contribution in PluginBoundary — one bad render doesn't sink the slot", async () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    await loadPlugin(
      definePlugin({
        name: "test.boundary",
        version: "1.0.0",
        setup: ({ host }) => {
          host.layout.register("test.boundary.slot", {
            id: "boom",
            order: 0,
            component: () => {
              throw new Error("boom");
            },
          });
          host.layout.register("test.boundary.slot", {
            id: "ok",
            order: 1,
            component: () => <span>still-here</span>,
          });
        },
      }),
    );
    render(<Slot name="test.boundary.slot" />);
    // The healthy contribution renders even though the other threw.
    expect(screen.getByText("still-here")).toBeTruthy();
    // The failure surfaces as the default boundary fallback.
    expect(screen.getByText(/failed to render/i)).toBeTruthy();
    spy.mockRestore();
  });

  it("emits a wrapping <div data-slot> when `wrapper` is set", async () => {
    await loadPlugin(
      definePlugin({
        name: "test.wrapper",
        version: "1.0.0",
        setup: ({ host }) => {
          host.layout.register("test.wrap", {
            id: "one",
            order: 0,
            component: () => <span>x</span>,
          });
        },
      }),
    );
    const { container } = render(<Slot name="test.wrap" wrapper className="foo" />);
    const wrapper = container.querySelector('[data-slot="test.wrap"]');
    expect(wrapper).not.toBeNull();
    expect(wrapper?.className).toBe("foo");
  });
});
