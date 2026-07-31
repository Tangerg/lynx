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
});
