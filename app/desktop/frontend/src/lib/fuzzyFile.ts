// Subsequence fuzzy match + ranking for file paths — the @file picker's
// matcher (T2.3). Scores higher for hits in the basename, consecutive runs,
// and segment/word starts, so "cmp" surfaces Composer.tsx above a deep
// incidental match. Small and dependency-free; the candidate set is the
// already-bounded workspace.listFiles result.

interface Hit {
  path: string;
  score: number;
}

/** Rank `paths` by fuzzy match against `query`, returning the top `limit`. An
 *  empty query returns the head of the list unranked (the picker just opened). */
export function fuzzyFile(query: string, paths: string[], limit: number): string[] {
  const q = query.toLowerCase();
  if (q === "") return paths.slice(0, limit);
  const hits: Hit[] = [];
  for (const path of paths) {
    const score = scorePath(q, path);
    if (score > 0) hits.push({ path, score });
  }
  hits.sort(
    (a, b) => b.score - a.score || a.path.length - b.path.length || a.path.localeCompare(b.path),
  );
  return hits.slice(0, limit).map((h) => h.path);
}

// A basename match dominates a path-spanning one: typing "comp" should rank
// Composer.tsx over a/b/c/o/m/p strewn across directory names.
function scorePath(q: string, path: string): number {
  const lower = path.toLowerCase();
  const base = lower.slice(lower.lastIndexOf("/") + 1);
  const baseScore = subseqScore(q, base);
  if (baseScore > 0) return baseScore + 1000;
  return subseqScore(q, lower);
}

// 0 if q isn't a subsequence of s; else a positive score rewarding consecutive
// matched chars and matches at a segment boundary (start / after / . _ -).
function subseqScore(q: string, s: string): number {
  let qi = 0;
  let score = 0;
  let prev = -2;
  for (let si = 0; si < s.length && qi < q.length; si++) {
    if (s[si] !== q[qi]) continue;
    let bonus = 1;
    if (si === prev + 1) bonus += 3;
    const before = s[si - 1];
    if (si === 0 || before === "/" || before === "." || before === "_" || before === "-")
      bonus += 2;
    score += bonus;
    prev = si;
    qi++;
  }
  return qi === q.length ? score : 0;
}
