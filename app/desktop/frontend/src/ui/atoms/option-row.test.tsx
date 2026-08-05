import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OptionRow } from "./option-row";

describe("OptionRow", () => {
  it("becomes an option, with its state, when a listbox drives selection by hand", () => {
    render(
      <>
        <OptionRow selected>picked</OptionRow>
        <OptionRow selected={false}>other</OptionRow>
      </>,
    );

    const picked = screen.getByText("picked");
    expect(picked.getAttribute("role")).toBe("option");
    expect(picked.getAttribute("aria-selected")).toBe("true");

    const other = screen.getByText("other");
    expect(other.getAttribute("aria-selected")).toBe("false");
  });

  // Without `selected`, the row must stay out of the way of whatever library owns
  // selection: emitting `aria-selected={undefined}` after the prop spread would erase
  // the value Base UI had just set.
  it("claims neither role nor selection state when it is not told", () => {
    render(<OptionRow>plain</OptionRow>);

    const row = screen.getByText("plain");
    expect(row.hasAttribute("role")).toBe(false);
    expect(row.hasAttribute("aria-selected")).toBe(false);
    expect(row.tagName).toBe("BUTTON");
  });
});
