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
