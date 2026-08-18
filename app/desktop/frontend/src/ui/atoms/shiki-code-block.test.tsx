import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { setCodeWrapPreference } from "@/lib/codeWrapPreference";
import { ShikiCodeBlock } from "./shiki-code-block";

const LONG_CODE = Array.from({ length: 30 }, (_, index) => `line ${index + 1}`).join("\n");

describe("ShikiCodeBlock", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  beforeEach(() => setCodeWrapPreference(false));

  it("keeps long code readable in its scrolling surface instead of replacing it with a fold row", async () => {
    const { container } = render(<ShikiCodeBlock lang="text" code={LONG_CODE} />);
    await act(async () => Promise.resolve());

    expect(container.querySelector(".shiki-body")?.textContent).toContain("line 30");
    expect(screen.queryByText("Show 30 lines")).toBeNull();
  });

  it("offers the Codex code-wrap toggle with pressed-state feedback", async () => {
    const { container } = render(<ShikiCodeBlock lang="text" code="a very long line" />);
    await act(async () => Promise.resolve());
    const toggle = screen.getByRole("button", { name: "Enable word wrap" });

    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(toggle);

    expect(
      screen.getByRole("button", { name: "Disable word wrap" }).getAttribute("aria-pressed"),
    ).toBe("true");
    expect(container.querySelector(".shiki-body")?.getAttribute("data-wrap")).toBe("true");
  });

  it("shares the Codex wrap preference across code blocks", async () => {
    const { container } = render(
      <>
        <ShikiCodeBlock lang="text" code="first long line" />
        <ShikiCodeBlock lang="text" code="second long line" />
      </>,
    );
    await act(async () => Promise.resolve());

    fireEvent.click(screen.getAllByRole("button", { name: "Enable word wrap" })[0]!);

    expect(screen.getAllByRole("button", { name: "Disable word wrap" })).toHaveLength(2);
    for (const body of container.querySelectorAll(".shiki-body")) {
      expect(body.getAttribute("data-wrap")).toBe("true");
    }
  });
});
