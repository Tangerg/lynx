import { describe, expect, it } from "vitest";
import { fuzzyFile } from "./fuzzyFile";

const FILES = [
  "src/components/chat/composer/Composer.tsx",
  "src/c/o/m/p/o/s/e/r.ts",
  "src/lib/utils.ts",
  "README.md",
  "src/components/common/Icon.tsx",
];

describe("fuzzyFile", () => {
  it("returns the head of the list for an empty query", () => {
    expect(fuzzyFile("", FILES, 3)).toEqual(FILES.slice(0, 3));
  });

  it("ranks a basename match above a path-spanning one", () => {
    const [top] = fuzzyFile("composer", FILES, 10);
    expect(top).toBe("src/components/chat/composer/Composer.tsx");
  });

  it("matches a basename subsequence", () => {
    expect(fuzzyFile("cmp", FILES, 10)).toContain("src/components/chat/composer/Composer.tsx");
  });

  it("excludes non-subsequence candidates", () => {
    expect(fuzzyFile("README", FILES, 10)).toEqual(["README.md"]);
    expect(fuzzyFile("zzzz", FILES, 10)).toEqual([]);
  });

  it("honors the limit", () => {
    expect(fuzzyFile("s", FILES, 2).length).toBeLessThanOrEqual(2);
  });
});
