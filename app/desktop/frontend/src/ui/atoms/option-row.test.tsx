import { render, screen } from "@testing-library/react";
import { Command } from "cmdk";
import { describe, expect, it } from "vitest";
import { FloatingSurface } from "./floating-surface";
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
    expect(picked.getAttribute("data-selected")).toBe("");

    const other = screen.getByText("other");
    expect(other.getAttribute("aria-selected")).toBe("false");
    expect(other.hasAttribute("data-selected")).toBe(false);
  });

  // Without `selected`, the row must stay out of the way of whatever library owns
  // selection: emitting `aria-selected={undefined}` after the prop spread would erase
  // the value cmdk had just set.
  it("claims neither role nor selection state when it is not told", () => {
    render(<OptionRow>plain</OptionRow>);

    const row = screen.getByText("plain");
    expect(row.hasAttribute("role")).toBe(false);
    expect(row.hasAttribute("aria-selected")).toBe(false);
    expect(row.tagName).toBe("BUTTON");
  });

  // The palette composes two libraries: cmdk owns behaviour, the design system owns the
  // panel and the row. The row join goes through `asChild`, so a version bump in either
  // could silently stop forwarding — and the palette has no other coverage.
  //
  // The panel WRAPS rather than slots: cmdk's root renders its own label element beside
  // the children, so `asChild` there has more than one child to slot onto and throws.
  it("composes with cmdk through asChild, keeping cmdk's own selection attributes", () => {
    render(
      <FloatingSurface>
        <Command>
          <Command.List>
            <Command.Item value="first" asChild>
              <OptionRow layout="flex" size="lg">
                first
              </OptionRow>
            </Command.Item>
            <Command.Item value="second" asChild>
              <OptionRow layout="flex" size="lg">
                second
              </OptionRow>
            </Command.Item>
          </Command.List>
        </Command>
      </FloatingSurface>,
    );

    const first = screen.getByText("first");
    expect(first.tagName).toBe("BUTTON");
    expect(first.getAttribute("cmdk-item")).toBe("");
    // cmdk starts on the first item, and the styling hooks it sets are the ones the row
    // already keys its selected wash off.
    expect(first.getAttribute("aria-selected")).toBe("true");
    expect(first.getAttribute("data-selected")).toBe("true");
    expect(screen.getByText("second").getAttribute("aria-selected")).toBe("false");
  });
});
