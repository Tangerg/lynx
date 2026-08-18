import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MermaidBlock } from "./MermaidBlock";

const renderMermaidSVG = vi.hoisted(() =>
  vi.fn(
    () =>
      '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120"><text>Graph</text></svg>',
  ),
);

vi.mock("beautiful-mermaid", () => ({
  renderMermaidSVG,
}));

const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  renderMermaidSVG.mockReset();
  renderMermaidSVG.mockReturnValue(
    '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120"><text>Graph</text></svg>',
  );
  writeText.mockClear();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
});

afterEach(() => {
  if (clipboardDescriptor) {
    Object.defineProperty(navigator, "clipboard", clipboardDescriptor);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
});

describe("MermaidBlock", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  it("announces the rendering placeholder", () => {
    render(<MermaidBlock code="graph TD; A-->B" />);

    expect(screen.getByRole("status", { name: "Loading Mermaid diagram" })).toBeTruthy();
  });

  it("renders a semantic diagram and copies its fenced source", async () => {
    const code = "graph TD; A-->B";
    render(<MermaidBlock code={code} />);

    expect(await screen.findByRole("img", { name: "Diagram" })).toBeTruthy();
    const copy = await screen.findByRole("button", { name: "Copy Mermaid" });
    fireEvent.click(copy);

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(`\`\`\`mermaid\n${code}\n\`\`\``));
  });

  it("falls back to the standard code surface after a settled parse error", async () => {
    renderMermaidSVG.mockImplementationOnce(() => {
      throw new Error("invalid diagram");
    });
    const { container } = render(<MermaidBlock code="not a graph" />);

    await waitFor(() => expect(container.querySelector(".shiki-block")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Copy code" })).toBeTruthy();
  });
});
