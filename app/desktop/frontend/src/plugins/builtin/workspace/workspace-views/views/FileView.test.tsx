import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const projection = vi.hoisted(() => {
  const highlighter = {
    codeToHtml: vi.fn(() => "<pre><code><span>highlighted</span></code></pre>"),
    getLoadedLanguages: vi.fn(() => ["go", "text"]),
  };
  return { highlighter, currentHighlighter: highlighter as typeof highlighter | null };
});

vi.mock("@/lib/highlight/useCodeHighlight", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/highlight/useCodeHighlight")>()),
  useCodeHighlighter: () => ({ highlighter: projection.currentHighlighter, theme: "test-theme" }),
}));

import { FileView } from "./FileView";

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;

beforeEach(() => {
  projection.currentHighlighter = projection.highlighter;
  projection.highlighter.codeToHtml.mockClear();
  projection.highlighter.getLoadedLanguages.mockClear();
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

describe("FileView syntax ownership", () => {
  it("highlights a file in the language determined by its path", () => {
    const content = "package main\nfunc main() {}";

    render(<FileView path="cmd/main.go" content={content} startLine={1} targetLine={0} />);

    expect(projection.highlighter.codeToHtml).toHaveBeenCalledWith(content, {
      lang: "go",
      theme: "test-theme",
    });
  });

  it("does not replay the target-line navigation when Shiki materializes", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    projection.currentHighlighter = null;
    const content = "package main\nfunc main() {}";
    const view = render(
      <FileView path="cmd/main.go" content={content} startLine={1} targetLine={2} />,
    );
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    projection.currentHighlighter = projection.highlighter;
    view.rerender(<FileView path="cmd/main.go" content={content} startLine={1} targetLine={2} />);

    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("keeps gutter and target identity for a window from the middle of a file", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const view = render(
      <FileView path="cmd/main.go" content={"first\nsecond"} startLine={40} targetLine={41} />,
    );

    expect(view.getByText("40")).toBeTruthy();
    expect(view.getByText("41")).toBeTruthy();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });
});
