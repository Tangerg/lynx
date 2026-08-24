import { describe, expect, it } from "vitest";

import {
  detectLocale,
  localeDefinitions,
  localeIDs,
  localeOptions,
} from "./locales";

describe("locale catalog", () => {
  it("publishes one exact, non-empty dictionary for every locale", () => {
    const canonical = Object.keys(localeDefinitions.en.messages).sort();
    expect(canonical).toHaveLength(1041);
    expect(localeOptions.map((option) => option.id)).toEqual(localeIDs);
    for (const locale of localeIDs) {
      const definition = localeDefinitions[locale];
      expect(Object.keys(definition.messages).sort()).toEqual(canonical);
      expect(
        Object.values(definition.messages).every(
          (message) => message.trim() !== "",
        ),
      ).toBe(true);
      expect(definition.nativeName.trim()).not.toBe("");
    }
    expect(localeDefinitions.ar.direction).toBe("rtl");
    expect(
      localeIDs.filter(
        (locale) => localeDefinitions[locale].direction === "rtl",
      ),
    ).toEqual(["ar"]);
  });

  it("detects script-specific Chinese before language fallbacks", () => {
    expect(detectLocale(["zh-Hant-HK", "en-US"])).toBe("zh-TW");
    expect(detectLocale(["zh-Hans", "en-US"])).toBe("zh-CN");
    expect(detectLocale(["ar-EG"])).toBe("ar");
    expect(detectLocale(["unknown"])).toBe("en");
  });
});
