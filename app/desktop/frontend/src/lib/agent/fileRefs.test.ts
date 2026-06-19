import { describe, expect, it } from "vitest";
import { parseFileRefs } from "./fileRefs";

describe("parseFileRefs", () => {
  it("extracts a path:line reference with surrounding text", () => {
    expect(parseFileRefs("see src/foo.go:42 now")).toEqual([
      "see ",
      { path: "src/foo.go", line: 42 },
      " now",
    ]);
  });

  it("matches a bare basename with a known extension", () => {
    expect(parseFileRefs("Composer.tsx")).toEqual([{ path: "Composer.tsx", line: 0 }]);
  });

  it("matches a slashed path without an extension", () => {
    expect(parseFileRefs("cmd/lyra/main")).toEqual([{ path: "cmd/lyra/main", line: 0 }]);
  });

  it("ignores prose abbreviations and versions", () => {
    expect(parseFileRefs("e.g. version 1.2.3 here")).toEqual(["e.g. version 1.2.3 here"]);
  });

  it("ignores an email address", () => {
    expect(parseFileRefs("mail a@b.com please")).toEqual(["mail a@b.com please"]);
  });

  it("drops the column but keeps the line", () => {
    expect(parseFileRefs("a/b.py:10:5")).toEqual([{ path: "a/b.py", line: 10 }]);
  });

  it("returns plain text unchanged when there's no reference", () => {
    expect(parseFileRefs("just words")).toEqual(["just words"]);
  });
});
