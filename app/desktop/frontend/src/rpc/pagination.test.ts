import { describe, expect, it, vi } from "vitest";
import { createAutoPagingPromise, PaginationError } from "./pagination";

function pager(pages: Array<{ data: string[]; nextCursor?: string }>) {
  return vi.fn(async (cursor?: string) => {
    const index = cursor ? Number(cursor.slice(1)) : 0;
    return pages[index]!;
  });
}

describe("auto-paging promise", () => {
  it("remains awaitable as the first wire page", async () => {
    const fetchPage = pager([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);

    await expect(createAutoPagingPromise(fetchPage)).resolves.toEqual({
      data: ["a"],
      nextCursor: "p1",
    });
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it("iterates every row and preserves the original cursor", async () => {
    const fetchPage = pager([
      { data: ["unused"] },
      { data: ["b"], nextCursor: "p2" },
      { data: ["c", "d"] },
    ]);
    const call = createAutoPagingPromise(fetchPage, "p1");

    const rows: string[] = [];
    for await (const row of call) rows.push(row);

    expect(rows).toEqual(["b", "c", "d"]);
    expect(fetchPage.mock.calls).toEqual([["p1"], ["p2"]]);
  });

  it("exposes full pages for page-level side data", async () => {
    const fetchPage = pager([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);
    const call = createAutoPagingPromise(fetchPage);
    const pages = [];

    for await (const page of call.pages()) pages.push(page);

    expect(pages).toEqual([{ data: ["a"], nextCursor: "p1" }, { data: ["b"] }]);
  });

  it("collects rows and supports early visitor termination", async () => {
    const fetchPage = pager([{ data: ["a", "b"], nextCursor: "p1" }, { data: ["c"] }]);
    const call = createAutoPagingPromise(fetchPage);

    await expect(call.autoPagingToArray()).resolves.toEqual(["a", "b", "c"]);

    const visited: string[] = [];
    await call.autoPagingEach((row) => {
      visited.push(row);
      return row !== "b";
    });
    expect(visited).toEqual(["a", "b"]);
  });

  it("rejects a repeated continuation instead of truncating silently", async () => {
    const fetchPage = vi.fn(async () => ({ data: ["a"], nextCursor: "same" }));
    const call = createAutoPagingPromise(fetchPage);

    const error = await call.autoPagingToArray().catch((reason: unknown) => reason);
    expect(error).toBeInstanceOf(PaginationError);
    expect(error).toEqual(expect.objectContaining({ cursor: "same" }));
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });
});
