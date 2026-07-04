import { afterEach, describe, expect, it } from "vitest";
import { findMessageRanges } from "./ranges";

afterEach(() => {
  document.body.replaceChildren();
});

describe("findMessageRanges", () => {
  it("finds case-insensitive matches only inside message content", () => {
    document.body.innerHTML = `
      <article class="msg-content">Alpha beta ALPHA</article>
      <aside>alpha outside chat</aside>
    `;

    const ranges = findMessageRanges("alpha");

    expect(ranges.map((range) => range.toString())).toEqual(["Alpha", "ALPHA"]);
  });

  it("treats regex syntax as literal text", () => {
    document.body.innerHTML = `<article class="msg-content">a+b aab a+b</article>`;

    const ranges = findMessageRanges("a+b");

    expect(ranges.map((range) => range.toString())).toEqual(["a+b", "a+b"]);
  });
});
