import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CodebaseCommandOwner } from "../application/codebaseCommandOwner";
import type { CodebaseGateway, CodebaseSearchHit } from "../application/ports/codebaseGateway";
import { CodebaseWorkspaceSurface } from "./codebase";

let owner: CodebaseCommandOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
});

describe("CodebaseWorkspaceSurface", () => {
  it("cannot materialize a retired workspace search after Session navigation", async () => {
    const retiredSearch = deferred<CodebaseSearchHit[]>();
    const search = vi.fn(() => retiredSearch.promise);
    owner = CodebaseCommandOwner.install({ search } as unknown as CodebaseGateway);
    const status = { state: "ready" as const, fileCount: 1, chunkCount: 1 };
    const view = render(<CodebaseWorkspaceSurface key="/one" cwd="/one" status={status} />);

    const input = screen.getByRole("searchbox");
    fireEvent.change(input, { target: { value: "owner" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await vi.waitFor(() =>
      expect(search).toHaveBeenCalledWith({ cwd: "/one", query: "owner", limit: 12 }),
    );

    view.rerender(<CodebaseWorkspaceSurface key="/two" cwd="/two" status={status} />);
    expect((screen.getByRole("searchbox") as HTMLInputElement).value).toBe("");

    await act(async () => {
      retiredSearch.resolve([
        { path: "old.ts", startLine: 1, endLine: 1, snippet: "old", score: 0.9 },
      ]);
      await retiredSearch.promise;
    });
    expect(screen.queryByText("old.ts:1")).toBeNull();
  });

  it("retires visible search material with its Runtime generation while preserving the query", async () => {
    const search = vi
      .fn()
      .mockResolvedValue([
        { path: "retired.ts", startLine: 7, endLine: 7, snippet: "retired", score: 0.9 },
      ] satisfies CodebaseSearchHit[]);
    owner = CodebaseCommandOwner.install({ search } as unknown as CodebaseGateway);
    const status = { state: "ready" as const, fileCount: 1, chunkCount: 1 };
    render(<CodebaseWorkspaceSurface cwd="/repo" status={status} />);

    const input = screen.getByRole("searchbox");
    fireEvent.change(input, { target: { value: "owner" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await screen.findByText("retired.ts:7-7");

    act(() => owner!.replaceRuntimeGeneration());

    expect((screen.getByRole("searchbox") as HTMLInputElement).value).toBe("owner");
    expect(screen.queryByText("retired.ts:7-7")).toBeNull();
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
