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

import { useSyncExternalStore } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import i18next from "i18next";

dayjs.extend(relativeTime);

// zh-cn is dynamic-imported on demand. Static side-effect imports of
// UMD locale files broke Vite's dev-mode dep-cache after multiple HMR
// cycles ("SyntaxError: Importing binding name 't' is not found");
// dynamic-import lets Vite split it into its own chunk that only
// downloads when a Chinese-speaking user is actually active.
let zhLoadingPromise: Promise<void> | null = null;
function ensureZhLoaded(): Promise<void> {
  if (!zhLoadingPromise) {
    zhLoadingPromise = import("dayjs/locale/zh-cn").then(() => undefined);
  }
  return zhLoadingPromise;
}

// External-store snapshot. Bumped whenever the dayjs locale actually
// changes (after a zh-cn load completes, or after en/zh switch). Lets
// React components subscribe via `useDayjsLocale()` and re-render
// without coupling to i18next's "language is `zh`" claim — they need
// to wait for the locale resources to actually land in dayjs.
let snapshot = 0;
const listeners = new Set<() => void>();
function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}
function bumpSnapshot(): void {
  snapshot += 1;
  for (const cb of listeners) cb();
}
export function useDayjsLocale(): number {
  return useSyncExternalStore(
    subscribe,
    () => snapshot,
    () => snapshot,
  );
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
      bumpSnapshot();
    });
  } else {
    try {
      dayjs.locale("en");
    } catch {
      /* unreachable — en is always built in. */
    }
    bumpSnapshot();
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
