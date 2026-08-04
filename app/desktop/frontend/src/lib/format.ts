// Shared token / cost formatting for the usage surfaces (composer chip, the
// session-cumulative header chip, the Usage settings pane). Extracted here once
// a third consumer appeared — one rule for how a token count / dollar amount
// reads across the app.

// Compact token count — 1234 → "1.2k", 1_200_000 → "1.2M". Whole thousands drop
// the ".0" ("12k", not "12.0k"); sub-1k stays exact.
export function fmtTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) {
    const k = n / 1000;
    return `${k % 1 === 0 ? k : k.toFixed(1)}k`;
  }
  return `${(n / 1_000_000).toFixed(1)}M`;
}

// USD amount. Sub-cent spend still reads as a real figure (4 dp) rather than
// rounding to "$0.00", which would imply free; everything else is 2 dp.
export function fmtCost(usd: number): string {
  if (usd > 0 && usd < 0.01) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

// Elapsed time, at the precision a reader can act on: sub-minute in seconds with
// one decimal under ten ("0.4s", "9.8s", "42s"), past a minute in m/s ("4m 06s").
// Deliberately not localized units — the surfaces that show this set them in mono
// beside token counts, where a translated "分" would break the column.
export function fmtDuration(ms: number): string {
  const seconds = ms / 1000;
  if (seconds < 10) return `${Math.round(seconds * 10) / 10}s`;
  // Rounded before the minute test, not after: 59.6s rounds to 60, and "60s" is a
  // reading no clock gives.
  const whole = Math.round(seconds);
  if (whole < 60) return `${whole}s`;
  const minutes = Math.floor(whole / 60);
  return `${minutes}m ${String(whole - minutes * 60).padStart(2, "0")}s`;
}
