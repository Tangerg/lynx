import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentActivityDisclosure } from "./activity-disclosure";

describe("AgentActivityDisclosure", () => {
  it("owns one accessible summary and detail region", () => {
    const onToggle = vi.fn();
    const { rerender } = render(
      <AgentActivityDisclosure
        icon="search"
        label="Search source"
        detail="app/runtime"
        open={false}
        onToggle={onToggle}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: /Search source/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region")).toBeNull();

    fireEvent.click(trigger);
    expect(onToggle).toHaveBeenCalledOnce();

    rerender(
      <AgentActivityDisclosure
        icon="search"
        label="Search source"
        detail="app/runtime"
        open
        onToggle={onToggle}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const region = screen.getByRole("region");
    expect(region.getAttribute("aria-labelledby")).toBe(trigger.id);
    expect(screen.getByText("Search result")).toBeTruthy();
  });

  it("keeps row actions outside the disclosure trigger", () => {
    const onToggle = vi.fn();
    const onAction = vi.fn();
    render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        label="Delegated run"
        open={false}
        onToggle={onToggle}
        actions={
          <button type="button" onClick={onAction}>
            Cancel
          </button>
        }
      >
        Child narrative
      </AgentActivityDisclosure>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onAction).toHaveBeenCalledOnce();
    expect(onToggle).not.toHaveBeenCalled();
  });

  // The three shells are the whole of the differentiation between a glance, a
  // product and trouble. Asserted on the material rather than on a class name: what
  // matters is that a line has no card under it and a flagged row has the tone's
  // edge, because both were true of nothing before and every row looked the same.
  it("gives each shell its own material", () => {
    const shells = (["line", "card", "flagged"] as const).map((shell) => {
      const { unmount } = render(
        <AgentActivityDisclosure
          icon="search"
          shell={shell}
          tone={shell === "flagged" ? "negative" : "neutral"}
          label={shell}
          open={false}
          onToggle={() => {}}
        >
          body
        </AgentActivityDisclosure>,
      );
      const row = screen.getByRole("button", { name: shell }).closest("[data-shell]");
      const classes = row?.className ?? "";
      const result = {
        shell: row?.getAttribute("data-shell"),
        filled: classes.includes("bg-card"),
        edged: classes.includes("border-negative-edge"),
      };
      unmount();
      return result;
    });

    expect(shells).toEqual([
      { shell: "line", filled: false, edged: false },
      { shell: "card", filled: true, edged: false },
      { shell: "flagged", filled: true, edged: true },
    ]);
  });

  // A caller that hands over its own leading mark owns that whole box: a plan's step
  // mark inside the glyph tray would be a mark inside a mark.
  it("frames its own glyph on a card, and never a caller's mark", () => {
    const { unmount } = render(
      <AgentActivityDisclosure icon="search" label="framed" open={false} onToggle={() => {}}>
        body
      </AgentActivityDisclosure>,
    );
    const framed = screen
      .getByRole("button", { name: "framed" })
      .querySelector("span[aria-hidden]");
    expect(framed?.className).toContain("bg-surface-2");
    unmount();

    render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        label="own"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const own = screen.getByRole("button", { name: /own/ }).querySelector("span[aria-hidden]");
    expect(own?.className).not.toContain("bg-surface-2");
  });

  it("keeps a line shell's identity glyph visible without card chrome", () => {
    render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="quiet search"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );

    const mark = screen
      .getByRole("button", { name: "quiet search" })
      .querySelector("span[aria-hidden]");
    expect(mark?.className).toContain("w-4");
    expect(mark?.className).not.toContain("hidden");
    expect(mark?.className).not.toContain("bg-surface-2");
    expect(mark?.querySelector("svg")).toBeTruthy();
  });

  it("puts the identity first and a quiet disclosure after the summary", () => {
    const { rerender } = render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Searched files"
        trailing="3 steps"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: /Searched files/ });
    const slots = Array.from(trigger.children)
      .map((child) => child.getAttribute("data-slot"))
      .filter(Boolean);
    expect(slots).toEqual([
      "agent-activity-mark",
      "agent-activity-label",
      "agent-activity-chevron",
    ]);
    const chevron = trigger.querySelector('[data-slot="agent-activity-chevron"]');
    expect(chevron?.getAttribute("class")).toContain("opacity-0");

    rerender(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Searched files"
        trailing="3 steps"
        open
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const openChevron = screen
      .getByRole("button", { name: /Searched files/ })
      .querySelector('[data-slot="agent-activity-chevron"]');
    expect(openChevron?.getAttribute("class")).toContain("opacity-100");
  });
});
