import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MermaidBlock } from "./MermaidBlock";

vi.mock("beautiful-mermaid", () => ({
  renderMermaidSVG: vi.fn(
    () =>
      '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120"><text>Graph</text></svg>',
  ),
}));

const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
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
});
