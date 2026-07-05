import { describe, expect, it } from "vitest";
import { commitApprovalArgs } from "./approvalArgsEditor";

describe("commitApprovalArgs", () => {
  it("returns undefined when the JSON value is unchanged", () => {
    expect(
      commitApprovalArgs(
        '{"path":"/tmp","flags":["a","b"]}',
        '{ "flags": ["a", "b"], "path": "/tmp" }',
      ),
    ).toBeUndefined();
  });

  it("returns the edited args object when the value changed", () => {
    expect(commitApprovalArgs('{"path":"/tmp"}', '{"path":"/safe","force":false}')).toEqual({
      path: "/safe",
      force: false,
    });
  });

  it("accepts an empty edited object", () => {
    expect(commitApprovalArgs('{"path":"/tmp"}', "{}")).toEqual({});
  });

  it("rejects malformed or non-object JSON", () => {
    expect(commitApprovalArgs("{}", "{")).toBeNull();
    expect(commitApprovalArgs("{}", "[]")).toBeNull();
    expect(commitApprovalArgs("{}", '"text"')).toBeNull();
  });
});
