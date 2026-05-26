// Localised time formatter. Goes through i18next for both the
// relative-time templates ("3 minutes ago" / "3 分钟前") and the
// absolute-date formats — sidesteps dayjs's own locale-loading dance
// (UMD subpath imports broke Vite's dep cache after multiple HMR
// cycles; the previous async-load + bumpSnapshot workaround left
// idle SessionRows stale on first paint).
//
// Threshold layout (mirrors dayjs's classic relativeTime cliffs):
//   < 45 s   → now
//   < 60 m   → X minutes
//   < 24 h   → X hours
//   < 7 d    → X days
//   same yr  → "MMM D" / "M月D日"
//   older    → "MMM D, YYYY" / "YYYY年M月D日"
//
// Components subscribe via `useT()` (already React-reactive on
// language change), so relative labels refresh on locale toggle
// without any custom store.

import dayjs from "dayjs";
import i18next from "i18next";

function isZh(): boolean {
  const lng = i18next.language ?? "en";
  return lng.startsWith("zh");
}

/**
 * Localised compact time label.
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  if (input === undefined || input === null || input === "") return "";
  const d = dayjs(input);
  if (!d.isValid()) return "";
  const now = dayjs();
  const diffSec = now.diff(d, "second");
  const diffMin = now.diff(d, "minute");
  const diffHour = now.diff(d, "hour");
  const diffDay = now.diff(d, "day");

  if (diffSec < 45) return i18next.t("time.now");
  if (diffMin < 60) return i18next.t("time.minutes", { count: diffMin });
  if (diffHour < 24) return i18next.t("time.hours", { count: diffHour });
  if (diffDay < 7) return i18next.t("time.days", { count: diffDay });

  // Absolute. Hand-format zh to avoid pulling in dayjs's zh-cn locale
  // bundle just for two month-name strings.
  const sameYear = d.year() === now.year();
  if (isZh()) {
    return sameYear
      ? `${d.month() + 1}月${d.date()}日`
      : `${d.year()}年${d.month() + 1}月${d.date()}日`;
  }
  return d.format(sameYear ? "MMM D" : "MMM D, YYYY");
}
