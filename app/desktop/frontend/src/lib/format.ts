// Shared token / cost / duration formatting for the usage surfaces (composer chip, the
// session-cumulative header chip, the Usage settings pane, run digests). Extracted here
// once a third consumer appeared — one rule for how a token count / dollar amount /
// elapsed time reads across the app.

import { activeLocale } from "./i18n";

// One place decides what a decimal point looks like. `toFixed` always writes a period,
// which is wrong in five of the eight locales this app ships — a German reader's "1.2k"
// is a thousand, not one and a fifth.
//
// What is deliberately NOT delegated to Intl:
//   * `notation: "compact"`, which would replace the k/M scale below with the locale's
//     own — and Japanese counts in 万, so a token readout would change magnitude words
//     and width depending on the language, in a mono column sized for neither.
//   * grouping, off everywhere here. These are compact readouts, never above three
//     digits before their unit, so grouping can only ever add a separator that widens
//     one column and not its neighbour.
//   * the unit itself (k / M / s / m / $). Those sit in a mono column beside each other,
//     and a translated "分" or a repositioned currency symbol breaks the alignment that
//     makes the column readable at a glance. The NUMBER follows the locale; the unit is
//     notation.
const formatters = new Map<string, Intl.NumberFormat>();

function decimal(value: number, fractionDigits: number, exact = false): string {
  const locale = activeLocale();
  const key = `${locale}/${fractionDigits}/${exact}`;
  let formatter = formatters.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, {
      useGrouping: false,
      minimumFractionDigits: exact ? fractionDigits : 0,
      maximumFractionDigits: fractionDigits,
    });
    formatters.set(key, formatter);
  }
  return formatter.format(value);
}

// Compact token count — 1234 → "1.2k", 1_200_000 → "1.2M". Whole thousands drop
// the ".0" ("12k", not "12.0k"); sub-1k stays exact.
export function fmtTokens(n: number): string {
  if (n < 1000) return decimal(n, 0);
  if (n < 1_000_000) return `${decimal(n / 1000, 1)}k`;
  return `${decimal(n / 1_000_000, 1, true)}M`;
}

// USD amount. Sub-cent spend still reads as a real figure (4 dp) rather than
// rounding to "$0.00", which would imply free; everything else is 2 dp.
export function fmtCost(usd: number): string {
  if (usd > 0 && usd < 0.01) return `$${decimal(usd, 4, true)}`;
  return `$${decimal(usd, 2, true)}`;
}

// Elapsed time, at the precision a reader can act on: sub-minute in seconds with
// one decimal under ten ("0.4s", "9.8s", "42s"), past a minute in m/s ("4m 06s").
export function fmtDuration(ms: number): string {
  const seconds = ms / 1000;
  if (seconds < 10) return `${decimal(Math.round(seconds * 10) / 10, 1)}s`;
  // Rounded before the minute test, not after: 59.6s rounds to 60, and "60s" is a
  // reading no clock gives.
  const whole = Math.round(seconds);
  if (whole < 60) return `${whole}s`;
  const minutes = Math.floor(whole / 60);
  return `${minutes}m ${String(whole - minutes * 60).padStart(2, "0")}s`;
}
