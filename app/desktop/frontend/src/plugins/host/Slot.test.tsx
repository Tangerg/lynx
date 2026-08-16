import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { definePlugin } from "../sdk";
import { Slot } from "./Slot";
import { contributeLayout } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

describe("slot", () => {
  it("renders nothing when no plugin has filled the slot", () => {
    const { container } = render(<Slot name="empty.slot" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders registered components ordered by `order`", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.layout.a",
        setup: (ctx) => {
          contributeLayout(ctx, "test.slot", {
            id: "a",
            order: 2,
            component: () => <span>A</span>,
          });
          contributeLayout(ctx, "test.slot", {
            id: "b",
            order: 1,
            component: () => <span>B</span>,
          });
        },
      }),
    );
    const { container } = render(<Slot name="test.slot" />);
    // Order=1 (B) comes before order=2 (A) regardless of registration sequence.
    expect(container.textContent).toBe("BA");
  });

  it("wraps each contribution in PluginBoundary — one bad render doesn't sink the slot", async () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    await loadPluginsForTest(
      definePlugin({
        name: "test.boundary",
        setup: (ctx) => {
          contributeLayout(ctx, "test.boundary.slot", {
            id: "boom",
            order: 0,
            component: () => {
              throw new Error("boom");
            },
          });
          contributeLayout(ctx, "test.boundary.slot", {
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
    await loadPluginsForTest(
      definePlugin({
        name: "test.wrapper",
        setup: (ctx) => {
          contributeLayout(ctx, "test.wrap", {
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
