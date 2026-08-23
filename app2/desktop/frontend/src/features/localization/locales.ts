import { arabicMessages } from "./messages/ar";
import { germanMessages } from "./messages/de";
import { englishMessages } from "./messages/en";
import { spanishMessages } from "./messages/es";
import { frenchMessages } from "./messages/fr";
import { japaneseMessages } from "./messages/ja";
import { koreanMessages } from "./messages/ko";
import { simplifiedChineseMessages } from "./messages/zh-CN";
import { traditionalChineseMessages } from "./messages/zh-TW";
import type { MessageDictionary } from "./messages/types";

export const localeIDs = [
  "en",
  "zh-CN",
  "zh-TW",
  "ja",
  "ko",
  "es",
  "fr",
  "de",
  "ar",
] as const;

export type Locale = (typeof localeIDs)[number];
export type TextDirection = "ltr" | "rtl";

interface LocaleDefinition {
  nativeName: string;
  direction: TextDirection;
  messages: MessageDictionary;
}

export const localeDefinitions = {
  en: { nativeName: "English", direction: "ltr", messages: englishMessages },
  "zh-CN": { nativeName: "简体中文", direction: "ltr", messages: simplifiedChineseMessages },
  "zh-TW": { nativeName: "繁體中文", direction: "ltr", messages: traditionalChineseMessages },
  ja: { nativeName: "日本語", direction: "ltr", messages: japaneseMessages },
  ko: { nativeName: "한국어", direction: "ltr", messages: koreanMessages },
  es: { nativeName: "Español", direction: "ltr", messages: spanishMessages },
  fr: { nativeName: "Français", direction: "ltr", messages: frenchMessages },
  de: { nativeName: "Deutsch", direction: "ltr", messages: germanMessages },
  ar: { nativeName: "العربية", direction: "rtl", messages: arabicMessages },
} as const satisfies Record<Locale, LocaleDefinition>;

export const localeOptions = localeIDs.map((id) => ({
  id,
  nativeName: localeDefinitions[id].nativeName,
  direction: localeDefinitions[id].direction,
}));

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && localeIDs.some((locale) => locale === value);
}

export function detectLocale(languages: readonly string[]): Locale {
  for (const language of languages) {
    const normalized = language.toLowerCase();
    if (normalized === "zh-tw" || normalized === "zh-hk" || normalized === "zh-mo" || normalized.startsWith("zh-hant")) {
      return "zh-TW";
    }
    if (normalized === "zh" || normalized.startsWith("zh-")) return "zh-CN";
    const direct = localeIDs.find((locale) => normalized === locale.toLowerCase() || normalized.startsWith(`${locale.toLowerCase()}-`));
    if (direct !== undefined) return direct;
  }
  return "en";
}
