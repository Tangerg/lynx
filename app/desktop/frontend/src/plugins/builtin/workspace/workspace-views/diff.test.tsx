import type { ReactNode } from "react";
import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceFileDiff } from "../application/diffViewModel";

const projection = vi.hoisted(() => ({
  fileFocus: { path: "src/a.ts", revision: 1 },
  files: [] as WorkspaceFileDiff[],
}));

const initialFiles: WorkspaceFileDiff[] = [
  { path: "src/a.ts", status: "modified", rows: [] },
  { path: "src/b.ts", status: "modified", rows: [] },
];

vi.mock("../application/diffViewModel", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/diffViewModel")>()),
  useWorkspaceDiffView: () => ({
    fileFocus: projection.fileFocus,
    files: projection.files,
    gitEnabled: true,
    isError: false,
    isLoading: false,
    notARepo: false,
    view: {
      files: projection.files,
      truncated: false,
    },
  }),
}));

vi.mock("@/ui", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/ui")>()),
  Segmented: () => <div />,
}));

vi.mock("@/ui/agent", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/ui/agent")>()),
  AgentViewNavigatorToggle: () => <button type="button" aria-label="toggle files" />,
  AgentViewSplit: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AgentWorkspaceView: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./views/DiffView", () => ({
  DiffView: () => <div />,
}));

vi.mock("./views/ReviewFileTree", () => ({
  ReviewFileTree: () => <div />,
}));

import { DiffWorkspaceSurface } from "./diff";

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;
const scrolledPaths: string[] = [];

beforeEach(() => {
  projection.fileFocus = { path: "src/a.ts", revision: 1 };
  projection.files = initialFiles;
  scrolledPaths.length = 0;
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = function scrollIntoView() {
    scrolledPaths.push(this.getAttribute("data-diff-file") ?? "");
  };
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

describe("DiffWorkspaceSurface", () => {
  it("locates every file navigation intent while the Diff view stays mounted", () => {
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual(["src/a.ts"]);

    act(() => {
      projection.fileFocus = { path: "src/b.ts", revision: 2 };
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts", "src/b.ts"]);
  });

  it("does not reinterpret a query replacement as a file navigation", () => {
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual(["src/a.ts"]);

    act(() => {
      projection.files = [...initialFiles];
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts"]);
  });

  it("keeps a focus intent pending until its file material arrives", () => {
    projection.files = [];
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual([]);

    act(() => {
      projection.files = initialFiles;
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts"]);
  });

  it("honours a repeated intent for the same file", () => {
    const view = render(<DiffWorkspaceSurface />);

    act(() => {
      projection.fileFocus = { path: "src/a.ts", revision: 2 };
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts", "src/a.ts"]);
  });
});
