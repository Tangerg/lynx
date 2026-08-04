import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { fileChangesViewModel } from "@/plugins/builtin/workspace/application/fileChangesViewModel";
import { FilesChanged } from "./FilesChanged";

const change = (path: string, over: Record<string, unknown> = {}) => ({
  path,
  change: "mod" as const,
  added: 12,
  removed: 3,
  ...over,
});

describe("FilesChanged", () => {
  // The point of the two-line row: the identifying half is not at the far end of a
  // truncating column any more.
  it("puts the basename on the row and the directory under it", () => {
    const view = fileChangesViewModel([change("src/plugins/builtin/chat/message/ui/Foo.tsx")]);
    render(<FilesChanged view={view} onSelect={vi.fn()} />);

    expect(screen.getByText("Foo.tsx")).toBeTruthy();
    expect(screen.getByText("src/plugins/builtin/chat/message/ui")).toBeTruthy();
  });

  it("has no second line for a root-level file rather than an empty one", () => {
    const view = fileChangesViewModel([change("README.md")]);
    const { container } = render(<FilesChanged view={view} onSelect={vi.fn()} />);

    expect(screen.getByText("README.md")).toBeTruthy();
    expect(container.textContent).not.toContain("undefined");
  });

  it("shows each row's figures, and a binary file's absence of them", () => {
    const view = fileChangesViewModel([
      change("a.ts", { added: 12, removed: 3 }),
      change("logo.png", { binary: true }),
    ]);
    render(<FilesChanged view={view} onSelect={vi.fn()} />);

    expect(screen.getByText("+12")).toBeTruthy();
    expect(screen.getByText("−3")).toBeTruthy();
    // The word, not a pair of zeroes claiming it changed nothing.
    expect(screen.getAllByText("bin").length).toBe(1);
  });
});
