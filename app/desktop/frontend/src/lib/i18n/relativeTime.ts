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

// Intl formatters are expensive to construct and are asked for once per message
// row, so they are cached per (locale, shape). Keyed on the locale so switching
// language rebuilds them instead of serving a stale one.
const dateTimeCache = new Map<string, Intl.DateTimeFormat>();

function dateTimeFormat(shape: string, opts: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const locale = bcp47();
  const key = `${locale}|${shape}`;
  const cached = dateTimeCache.get(key);
  if (cached) return cached;
  const created = new Intl.DateTimeFormat(locale, opts);
  dateTimeCache.set(key, created);
  return created;
}

function relative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  return new Intl.RelativeTimeFormat(bcp47(), { numeric: "auto" }).format(value, unit);
}

function absolute(d: Date, sameYear: boolean): string {
  return sameYear
    ? dateTimeFormat("md", { month: "short", day: "numeric" }).format(d)
    : dateTimeFormat("ymd", { year: "numeric", month: "short", day: "numeric" }).format(d);
}

function parse(input: string | number | Date | undefined | null): Date | null {
  if (input === undefined || input === null || input === "") return null;
  const d = input instanceof Date ? input : new Date(input);
  return Number.isNaN(d.getTime()) ? null : d;
}

/**
 * Absolute date + time — for turn separators, schedule rows and exported
 * transcripts. The year appears only when it isn't this one, and the 12- vs
 * 24-hour choice comes from the app locale rather than individual callsites or
 * the host OS locale.
 *
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatDateTime(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  const sameYear = d.getFullYear() === new Date().getFullYear();
  return dateTimeFormat(sameYear ? "md-hm" : "ymd-hm", {
    ...(sameYear ? {} : { year: "numeric" }),
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(d);
}

/**
 * Clock time alone — for a turn's caption, where the date is carried once by the
 * day separator above it and repeating it on every turn is noise. 12- vs 24-hour
 * comes from the locale, as everywhere else here.
 *
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatClock(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  return dateTimeFormat("hm", { hour: "numeric", minute: "2-digit" }).format(d);
}

/**
 * The calendar day a timestamp falls on, in the local zone, as `YYYY-MM-DD`.
 *
 * An identity for grouping, never shown: comparing formatted labels would make
 * the grouping depend on the display locale, and comparing ISO strings would
 * group by UTC day, which is the wrong midnight for everyone west of it.
 */
export function dayKey(input: string | number | Date | undefined | null): string | null {
  const d = parse(input);
  if (!d) return null;
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/** The date without the clock — the day separator's label. */
export function formatDay(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  const sameYear = d.getFullYear() === new Date().getFullYear();
  return absolute(d, sameYear);
}

/**
 * Localised compact time label.
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";

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
