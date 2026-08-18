import { fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentDockTabs, type AgentDockTab } from "./context-dock";

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;

beforeEach(() => {
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

function tabs(activeId: string): AgentDockTab[] {
  return ["explorer", "file", "diff", "terminal", "plan", "timeline"].map((id) => ({
    id,
    title: id,
    active: id === activeId,
  }));
}

describe("AgentDockTabs", () => {
  it("brings a newly active overflow tab into the visible strip", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const view = render(<AgentDockTabs tabs={tabs("explorer")} ariaLabel="Workspace tabs" />);
    scrollIntoView.mockClear();

    view.rerender(<AgentDockTabs tabs={tabs("timeline")} ariaLabel="Workspace tabs" />);

    expect(scrollIntoView).toHaveBeenCalledOnce();
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
  });

  it("exposes which overflow edges still contain hidden tabs", () => {
    const view = render(<AgentDockTabs tabs={tabs("explorer")} ariaLabel="Workspace tabs" />);
    const strip = view.container.querySelector<HTMLElement>(".agent-dock-tabs")!;
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 300 },
      scrollWidth: { configurable: true, value: 600 },
    });

    strip.scrollLeft = 0;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(false);
    expect(strip.hasAttribute("data-overflow-end")).toBe(true);

    strip.scrollLeft = 120;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(true);
    expect(strip.hasAttribute("data-overflow-end")).toBe(true);

    strip.scrollLeft = 300;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(true);
    expect(strip.hasAttribute("data-overflow-end")).toBe(false);
  });
});
