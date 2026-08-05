import { describe, expect, it } from "vitest";
import { toolFields } from "./projections";

// A write's card body was blank: no diff rows to diff a new file against, and
// `argsText` gives a non-generic tool no argument text — so the content the call
// carried reached the view through no route at all.
const write = (content: unknown) =>
  ({ name: "write", arguments: { path: "a/b.md", content } }) as Parameters<typeof toolFields>[0];

describe("what a write projects", () => {
  it("carries the head of what it wrote, and the count it omits", () => {
    const lines = Array.from({ length: 45 }, (_, i) => `line ${i + 1}`);

    const fields = toolFields(write(lines.join("\n")));

    expect(fields.written).toHaveLength(40);
    expect(fields.written?.[0]).toBe("line 1");
    expect(fields.writtenLines).toBe(45);
  });

  it("drops the trailing newline a file ends with rather than counting a blank line", () => {
    const fields = toolFields(write("one\ntwo\n"));

    expect(fields.written).toEqual(["one", "two"]);
    expect(fields.writtenLines).toBe(2);
  });

  it("carries nothing when the call carried nothing", () => {
    expect(toolFields(write("")).written).toBeUndefined();
    expect(toolFields(write(undefined)).written).toBeUndefined();
    expect(toolFields(write({ a: 1 })).written).toBeUndefined();
  });
});
