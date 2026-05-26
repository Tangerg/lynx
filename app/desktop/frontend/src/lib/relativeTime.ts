// Localised time formatter. Wraps dayjs's `relativeTime` plugin and
// syncs its locale to the app's i18n locale (en / zh-cn) so a
// timestamp reads as "3 minutes ago" / "3 分钟前" depending on UI
// language.
//
// Threshold strategy:
//   < 7 days  → relative natural language ("3 minutes ago" / "3 分钟前")
//   same year → absolute month/day ("Mar 5" / "3月5日")
//   older     → full date ("Mar 5, 2025" / "2025年3月5日")
//
// The 7-day cliff disambiguates `m` (relative would be "Xm" / "X 分钟",
// absolute path uses spelled month names) and gives older sessions a
// scannable real date instead of "23 days ago" / "2 个月前".

import dayjs from "dayjs";
import "dayjs/locale/zh-cn";
import relativeTime from "dayjs/plugin/relativeTime";
import i18next from "i18next";

dayjs.extend(relativeTime);

function syncDayjsLocale(lng: string | undefined): void {
  dayjs.locale(lng && lng.startsWith("zh") ? "zh-cn" : "en");
}

// Sync immediately + on every language switch. Subscribing once at
// module load is enough — there's exactly one i18next instance.
syncDayjsLocale(i18next.language);
i18next.on("languageChanged", syncDayjsLocale);

function isZh(): boolean {
  return dayjs.locale().startsWith("zh");
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
  const diffDays = now.diff(d, "day");
  if (diffDays < 7) return d.fromNow();
  const sameYear = d.year() === now.year();
  if (isZh()) return d.format(sameYear ? "M月D日" : "YYYY年M月D日");
  return d.format(sameYear ? "MMM D" : "MMM D, YYYY");
}
