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
import relativeTime from "dayjs/plugin/relativeTime";
import i18next from "i18next";

dayjs.extend(relativeTime);

// zh-cn is dynamic-imported on demand. Static side-effect imports of
// UMD locale files have caused HMR / Vite dep-cache headaches; the
// dynamic-import lets Vite handle it as its own chunk and the locale
// only loads when a Chinese-speaking user actually opens the app.
let zhLoadingPromise: Promise<void> | null = null;
function ensureZhLoaded(): Promise<void> {
  if (!zhLoadingPromise) {
    zhLoadingPromise = import("dayjs/locale/zh-cn").then(() => undefined);
  }
  return zhLoadingPromise;
}

function applyLocale(lng: string | undefined): void {
  const wantZh = !!lng && lng.startsWith("zh");
  if (wantZh) {
    void ensureZhLoaded().then(() => {
      try {
        dayjs.locale("zh-cn");
      } catch {
        /* zh-cn not registered for some reason; stay on current. */
      }
    });
  } else {
    try {
      dayjs.locale("en");
    } catch {
      /* unreachable — en is always built in. */
    }
  }
}

// Lazy bind on first formatRelative() call (when React is mounted +
// i18next is initialised). Subscribing at module load was brittle
// because relativeTime.ts imports `i18next` directly rather than via
// `@/lib/i18n`, so there's no init-order guarantee.
let bound = false;
function ensureBound(): void {
  if (bound) return;
  bound = true;
  applyLocale(i18next.language);
  try {
    i18next.on("languageChanged", applyLocale);
  } catch {
    /* leave dayjs on whatever applyLocale managed to set. */
  }
}

function isZh(): boolean {
  return dayjs.locale().startsWith("zh");
}

/**
 * Localised compact time label.
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  if (input === undefined || input === null || input === "") return "";
  ensureBound();
  const d = dayjs(input);
  if (!d.isValid()) return "";
  const now = dayjs();
  const diffDays = now.diff(d, "day");
  if (diffDays < 7) return d.fromNow();
  const sameYear = d.year() === now.year();
  if (isZh()) return d.format(sameYear ? "M月D日" : "YYYY年M月D日");
  return d.format(sameYear ? "MMM D" : "MMM D, YYYY");
}
