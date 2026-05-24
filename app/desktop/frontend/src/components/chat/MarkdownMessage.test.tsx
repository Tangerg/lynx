import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { MarkdownMessage } from "./MarkdownMessage";

// MarkdownMessage now renders through react-markdown + remark-gfm with our
// rehype-fade-in plugin and a Shiki/Mermaid component map. These tests
// cover the high-value invariants:
//
//   1. Streaming partials never throw or hang.
//   2. Unmatched markers (`` ` ``, `**`, ` ``` `) get auto-closed by
//      closeOpenMarkers so the model's mid-flight tokens already look
//      "right" while the closer streams in.
//   3. Block-level constructs (fenced code, lists, headings) render via
//      react-markdown rather than as raw backticks / hashes.
//   4. Each non-code text node gets per-word `<span class="fade-in">`
//      wrappers from rehypeFadeIn.
//   5. With `instant`, no `.fade-in` wrappers are produced.

describe("MarkdownMessage", () => {
  it("renders an empty string without throwing", () => {
    const { container } = render(<MarkdownMessage text="" />);
    // react-markdown adds a wrapper div; the body is just empty.
    expect(container.querySelector(".md")).toBeTruthy();
  });

  it("wraps plain words in .fade-in spans", () => {
    const { container } = render(<MarkdownMessage text="Hello world" />);
    const spans = container.querySelectorAll("span.fade-in");
    expect(spans.length).toBeGreaterThanOrEqual(2);
    const text = Array.from(spans)
      .map((s) => s.textContent)
      .join("|");
    expect(text).toContain("Hello");
    expect(text).toContain("world");
  });

  it("auto-closes an unmatched inline backtick so it renders as code", () => {
    const { container } = render(<MarkdownMessage text="use `code" />);
    // closeOpenMarkers appends the missing backtick → react-markdown
    // emits an inline <code> element.
    expect(container.querySelector("code")).toBeTruthy();
    expect(container.textContent ?? "").toContain("code");
  });

  it("auto-closes an unmatched `**` so it renders as strong", () => {
    const { container } = render(<MarkdownMessage text="that **really" />);
    expect(container.querySelector("strong")).toBeTruthy();
    expect(container.textContent ?? "").toContain("really");
  });

  it("renders a complete fenced code block as a Shiki block", () => {
    const src = "before\n```js\nconst x = 1;\n```\nafter";
    const { container } = render(<MarkdownMessage text={src} />);
    expect(container.querySelector(".shiki-block")).toBeTruthy();
    expect(container.textContent ?? "").toContain("const x = 1;");
  });

  it("renders a streaming-partial code block (no closer yet)", () => {
    // closeOpenMarkers should synthesise the closing fence so this
    // already shows up as a code block.
    const src = "```js\nconst x";
    const { container } = render(<MarkdownMessage text={src} />);
    expect(container.querySelector(".shiki-block")).toBeTruthy();
    expect(container.textContent ?? "").toContain("const x");
  });

  it("renders GFM tables via remark-gfm", () => {
    const src = "| a | b |\n|---|---|\n| 1 | 2 |";
    const { container } = render(<MarkdownMessage text={src} />);
    expect(container.querySelector("table")).toBeTruthy();
    expect(container.querySelectorAll("td").length).toBe(2);
  });

  it("renders markdown lists", () => {
    const src = "- one\n- two\n- three";
    const { container } = render(<MarkdownMessage text={src} />);
    expect(container.querySelector("ul")).toBeTruthy();
    expect(container.querySelectorAll("li").length).toBe(3);
  });

  it("drops fade-in wrappers when `instant` is true", () => {
    const { container } = render(<MarkdownMessage text="Hello world" instant />);
    expect(container.querySelectorAll("span.fade-in").length).toBe(0);
    expect(container.textContent ?? "").toContain("Hello world");
  });
});
