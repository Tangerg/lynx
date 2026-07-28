import { describe, expect, it, vi } from "vitest";
import type { Page } from "./wire.generated";
import { collectPages, eachPage } from "./pagination";

function pager<T>(pages: T[][]): (cursor?: string) => Promise<Page<T>> {
  return (cursor) => {
    const index = cursor ? Number(cursor) : 0;
    const data = pages[index] ?? [];
    const next = index + 1 < pages.length ? String(index + 1) : undefined;
    return Promise.resolve(next ? { data, nextCursor: next } : { data });
  };
}

describe("collectPages", () => {
  it("follows the cursor to the end", async () => {
    expect(await collectPages(pager([["a", "b"], ["c"], ["d"]]))).toEqual(["a", "b", "c", "d"]);
  });

  it("asks once when the runtime hands back no cursor", async () => {
    const fetchPage = vi.fn(pager([["only"]]));

    expect(await collectPages(fetchPage)).toEqual(["only"]);
    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(fetchPage).toHaveBeenCalledWith(undefined);
  });

  it("stops on a cursor that doesn't advance instead of looping forever", async () => {
    // A server that keeps handing back the same cursor is broken; hanging the
    // caller on it would be worse than returning what we have.
    const fetchPage = vi.fn().mockResolvedValue({ data: ["x"], nextCursor: "same" });

    expect(await collectPages(fetchPage)).toEqual(["x", "x"]);
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });
});

describe("eachPage", () => {
  it("hands over whole pages, so a caller can read the fields beside `data`", async () => {
    const pages = [
      { data: [1], runs: ["r1"], nextCursor: "1" },
      { data: [2], runs: ["r1", "r2"] },
    ];
    const seen: string[][] = [];

    await eachPage(
      (cursor) => Promise.resolve(pages[cursor ? Number(cursor) : 0]!),
      (page) => seen.push(page.runs),
    );

    expect(seen).toEqual([["r1"], ["r1", "r2"]]);
  });
});
