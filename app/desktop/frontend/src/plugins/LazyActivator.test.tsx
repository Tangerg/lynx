import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { createElement } from "react";
import { makeLazyActivator } from "./LazyActivator";

describe("makeLazyActivator", () => {
  it("renders the 'Activating <label>…' placeholder", () => {
    const Placeholder = makeLazyActivator("Diff", () => {});
    render(createElement(Placeholder));
    expect(screen.getByText("Activating Diff…")).toBeTruthy();
  });

  it("calls onActivate exactly once on mount", () => {
    const onActivate = vi.fn();
    const Placeholder = makeLazyActivator("X", onActivate);
    const { unmount, rerender } = render(createElement(Placeholder));
    rerender(createElement(Placeholder));
    expect(onActivate).toHaveBeenCalledOnce();
    unmount();
  });

  it("attaches the .lazy-activator class on the wrapper for CSS styling", () => {
    const Placeholder = makeLazyActivator("Tools", () => {});
    const { container } = render(createElement(Placeholder));
    expect(container.querySelector(".lazy-activator")).not.toBeNull();
  });
});
