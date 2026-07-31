import { afterEach, describe, expect, it } from "vitest";
import { activeLocale, setLocale } from "./index";

afterEach(async () => {
  setLocale("en");
  await Promise.resolve();
});

describe("locale selection identity", () => {
  it("preserves the requested locale while its lazy dictionary falls back to English", async () => {
    setLocale("fixture-locale-without-a-bundle");
    await Promise.resolve();

    expect(activeLocale()).toBe("fixture-locale-without-a-bundle");
  });
});
