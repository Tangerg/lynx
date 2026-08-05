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
        mark="glyph"
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
        mark="glyph"
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
});
