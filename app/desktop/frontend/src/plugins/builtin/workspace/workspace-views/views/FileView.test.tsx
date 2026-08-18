import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const highlighter = vi.hoisted(() => ({
  codeToHtml: vi.fn(() => "<pre><code><span>highlighted</span></code></pre>"),
  getLoadedLanguages: vi.fn(() => ["go", "text"]),
}));

vi.mock("@/lib/highlight/useCodeHighlight", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/highlight/useCodeHighlight")>()),
  useCodeHighlighter: () => ({ highlighter, theme: "test-theme" }),
}));

import { FileView } from "./FileView";

beforeEach(() => {
  highlighter.codeToHtml.mockClear();
  highlighter.getLoadedLanguages.mockClear();
});

describe("FileView syntax ownership", () => {
  it("highlights a file in the language determined by its path", () => {
    const content = "package main\nfunc main() {}";

    render(<FileView path="cmd/main.go" content={content} targetLine={0} />);

    expect(highlighter.codeToHtml).toHaveBeenCalledWith(content, {
      lang: "go",
      theme: "test-theme",
    });
  });
});
