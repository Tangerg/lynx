import { describe, expect, it } from "vitest";
import { langFromPath } from "./shiki";

describe("langFromPath", () => {
  it("maps common extensions to their Shiki language", () => {
    expect(langFromPath("a/b/main.go")).toBe("go");
    expect(langFromPath("script.py")).toBe("python");
    expect(langFromPath("src/App.tsx")).toBe("tsx");
    expect(langFromPath("lib/util.ts")).toBe("typescript");
    expect(langFromPath("main.rs")).toBe("rust");
    expect(langFromPath("style.scss")).toBe("scss");
  });

  it("recognizes well-known bare filenames", () => {
    expect(langFromPath("deploy/Dockerfile")).toBe("dockerfile");
    expect(langFromPath("Makefile")).toBe("bash");
  });

  it("falls back to text for unknown / extensionless paths", () => {
    expect(langFromPath("notes")).toBe("text");
    expect(langFromPath("data.kdl")).toBe("text");
  });

  it("is case-insensitive on the extension", () => {
    expect(langFromPath("README.MD")).toBe("markdown");
  });
});
