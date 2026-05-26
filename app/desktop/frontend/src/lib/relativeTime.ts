// Compact relative-time formatter. Wraps dayjs + its `relativeTime`
// plugin with a tightened locale so labels in tight surfaces (sidebar
// session rows, status pills) read as `now / 3m / 1h / 1d / 2w` instead
// of the default "3 minutes ago" / "a day ago" prose.
//
// dayjs handles the diff math + thresholds (45s → minute, 22h → day,
// etc.); we just override the label strings.

import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import updateLocale from "dayjs/plugin/updateLocale";

dayjs.extend(relativeTime);
dayjs.extend(updateLocale);

// Single tight locale for every "X ago" surface in the app. Future
// columns are unused for our case (we never format upcoming times)
// but kept so dayjs doesn't fall back to the default English string.
dayjs.updateLocale("en", {
  relativeTime: {
    future: "in %s",
    past: "%s",
    s: "now",
    m: "1m",
    mm: "%dm",
    h: "1h",
    hh: "%dh",
    d: "1d",
    dd: "%dd",
    M: "1mo",
    MM: "%dmo",
    y: "1y",
    yy: "%dy",
  },
});

/**
 * Compact relative-time string for an ISO timestamp / Date / dayjs input.
 * Returns "" for unparseable input so the caller can render a fallback.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  if (input === undefined || input === null || input === "") return "";
  const d = dayjs(input);
  if (!d.isValid()) return "";
  return d.fromNow(true /* without suffix */);
}
