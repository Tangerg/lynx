// Localised compact time formatter. Uses the browser-native
// `Intl.RelativeTimeFormat` + `Intl.DateTimeFormat` — no library
// needed. Both APIs handle plurals + locale strings natively, which
// is the whole point: "3 minutes ago" / "3 分钟前", "yesterday" /
// "昨天", "Mar 5" / "3月5日".
//
// Threshold layout:
//   < 60 s   → "now" / "现在"
//   < 60 m   → X minute(s) ago
//   < 24 h   → X hour(s) ago
//   < 7 d    → X day(s) ago (1 day → "yesterday"/"昨天" via numeric:auto)
//   same yr  → "MMM D" / "M月D日"
//   older    → "MMM D, YYYY" / "YYYY年M月D日"
//
// Components subscribe via `useT()` (already React-reactive on
// language change), so labels refresh on locale toggle.

import i18next from "i18next";

// Translate i18next's locale id to a BCP-47 tag Intl expects.
// "zh" / "zh-TW" become "zh-CN" / "zh-TW" explicitly so ICU picks
// the right grammar variant (Simplified vs Traditional). All other
// locales are passed through — they already are BCP-47 primary
// subtags.
export function bcp47(): string {
  const lng = i18next.language ?? "en";
  if (lng === "zh") return "zh-CN";
  if (lng === "zh-TW" || lng.toLowerCase() === "zh-tw") return "zh-TW";
  return lng;
}

function relative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  return new Intl.RelativeTimeFormat(bcp47(), { numeric: "auto" }).format(value, unit);
}

function absolute(d: Date, sameYear: boolean): string {
  const opts: Intl.DateTimeFormatOptions = sameYear
    ? { month: "short", day: "numeric" }
    : { year: "numeric", month: "short", day: "numeric" };
  return new Intl.DateTimeFormat(bcp47(), opts).format(d);
}

/**
 * Localised compact time label.
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  if (input === undefined || input === null || input === "") return "";
  const d = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(d.getTime())) return "";

  const now = Date.now();
  const diffMs = now - d.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  // Under a minute reads as "now" / "现在". Intl's `numeric: "auto"` only emits
  // "now" for value=0, so collapse the whole sub-minute window to 0 — a 45s
  // cliff left 45–59s falling into the minute branch as a stray "this minute".
  if (diffSec < 60) return relative(0, "second");
  if (diffMin < 60) return relative(-diffMin, "minute");
  if (diffHour < 24) return relative(-diffHour, "hour");
  if (diffDay < 7) return relative(-diffDay, "day");

  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return absolute(d, sameYear);
}
