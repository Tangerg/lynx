import { describe, expect, it } from "vitest";
import {
  escapeCurrencyDollars,
  normalizeMarkdownMath,
  rewriteCustomMathTags,
  rewriteLatexBracketDelimiters,
} from "./preprocess";

describe("rewriteLatexBracketDelimiters", () => {
  it("rewrites single-backslash inline `\\(...\\)` to `$...$`", () => {
    expect(rewriteLatexBracketDelimiters("a \\(x^2\\) b")).toBe("a $x^2$ b");
  });

  it("rewrites double-backslash inline `\\\\(...\\\\)` to `$...$`", () => {
    expect(rewriteLatexBracketDelimiters("a \\\\(x^2\\\\) b")).toBe("a $x^2$ b");
  });

  it("rewrites single-backslash display `\\[...\\]` to `$$...$$`", () => {
    expect(rewriteLatexBracketDelimiters("\\[a+b\\]")).toBe("$$a+b$$");
  });

  it("rewrites double-backslash display `\\\\[...\\\\]` to `$$...$$`", () => {
    expect(rewriteLatexBracketDelimiters("\\\\[a+b\\\\]")).toBe("$$a+b$$");
  });

  it("trims whitespace inside the delimiters", () => {
    expect(rewriteLatexBracketDelimiters("\\(  x  \\)")).toBe("$x$");
  });

  it("leaves a half-arrived opener untouched (streaming-safe)", () => {
    expect(rewriteLatexBracketDelimiters("partial \\(x^2")).toBe("partial \\(x^2");
  });
});

describe("rewriteCustomMathTags", () => {
  it("rewrites `[/math]...[/math]` to `$$...$$`", () => {
    expect(rewriteCustomMathTags("[/math]a+b[/math]")).toBe("$$a+b$$");
  });

  it("rewrites `[/inline]...[/inline]` to `$...$`", () => {
    expect(rewriteCustomMathTags("x [/inline]y[/inline] z")).toBe("x $y$ z");
  });

  it("leaves an unterminated tag untouched (streaming-safe)", () => {
    expect(rewriteCustomMathTags("[/math]a+b")).toBe("[/math]a+b");
  });
});

describe("escapeCurrencyDollars", () => {
  it("escapes a `$` before a digit", () => {
    expect(escapeCurrencyDollars("it costs $5 today")).toBe("it costs \\$5 today");
  });

  it("escapes decimal + grouped amounts", () => {
    expect(escapeCurrencyDollars("$19.99 and $1,299")).toBe("\\$19.99 and \\$1,299");
  });

  it("escapes a leading `$5` at start of string", () => {
    expect(escapeCurrencyDollars("$5")).toBe("\\$5");
  });

  it("preserves `$$` display-math delimiters", () => {
    expect(escapeCurrencyDollars("$$5x$$")).toBe("$$5x$$");
  });

  it("does not double-escape an already-escaped `\\$5`", () => {
    expect(escapeCurrencyDollars("\\$5")).toBe("\\$5");
  });

  it("escapes across an even backslash run (the `$` is unescaped there)", () => {
    // `\\` is a literal backslash, so the `$` after it is NOT escaped yet.
    expect(escapeCurrencyDollars("\\\\$5")).toBe("\\\\\\$5");
  });

  it("leaves `$` before a non-digit alone (likely math)", () => {
    expect(escapeCurrencyDollars("$x$")).toBe("$x$");
  });
});

describe("normalizeMarkdownMath (composition)", () => {
  it("normalizes bracket math while escaping neighbouring currency", () => {
    expect(normalizeMarkdownMath("pay $5 for \\(x^2\\)")).toBe("pay \\$5 for $x^2$");
  });

  it("keeps digit-opening bracket math correct (escape runs before rewrite)", () => {
    expect(normalizeMarkdownMath("\\(5x\\)")).toBe("$5x$");
  });

  it("handles custom tags and currency together", () => {
    expect(normalizeMarkdownMath("[/inline]5x[/inline] costs $5")).toBe("$5x$ costs \\$5");
  });

  it("is a no-op for plain prose", () => {
    expect(normalizeMarkdownMath("just some text")).toBe("just some text");
  });
});
