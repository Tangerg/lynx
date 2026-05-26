// Compact time formatter for tight surfaces (sidebar session rows,
// status pills). Wraps dayjs with a tightened locale for relative
// units and falls through to absolute month/year format once a
// timestamp is older than a week.
//
// Threshold layout:
//   < 1 min   → "now"
//   < 1 hour  → "Xm"      (m always = minute — no ambiguity)
//   < 1 day   → "Xh"
//   < 7 days  → "Xd"
//   < 1 year  → "MMM D"   (e.g. "Mar 5") — absolute, never "mo"
//   ≥ 1 year  → "MMM YYYY"
//
// The cliff at 7d keeps `m` reserved for minutes; month never collides
// because we switch to spelled-out absolute dates before dayjs would
// emit "Xmo".

import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import updateLocale from "dayjs/plugin/updateLocale";

dayjs.extend(relativeTime);
dayjs.extend(updateLocale);

// `s/m/h/d` cover what we use (everything ≥ 7d goes through the
// absolute path below). `M`/`y` rows kept defaulted just so dayjs
// doesn't throw if a caller asks for `.fromNow()` directly.
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
 * Compact time label for an ISO timestamp / Date / dayjs input.
 * Recent → relative ("3m", "1d"); older → absolute month/year ("Mar 5",
 * "Mar 2025"). Returns "" for unparseable input.
 */
export function formatRelative(input: string | number | Date | undefined | null): string {
  if (input === undefined || input === null || input === "") return "";
  const d = dayjs(input);
  if (!d.isValid()) return "";
  const now = dayjs();
  const diffDays = now.diff(d, "day");
  if (diffDays < 7) return d.fromNow(true /* without suffix */);
  if (d.year() === now.year()) return d.format("MMM D");
  return d.format("MMM YYYY");
}
