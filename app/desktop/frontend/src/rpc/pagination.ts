import type { Page } from "./shapes";

/**
 * Read a paged method to the end.
 *
 * `Page.data` is ONE page. The runtime clamps `sessions.list` to 100, `items.list`
 * to 200 and `runs.list` / `runs.listOpenInterrupts` / `schedules.list` to 100 —
 * and a client cannot opt out, because a `limit` above the cap is clamped back
 * down. A non-empty `nextCursor` is the protocol's "there is more" signal (§4.11);
 * the server documents that it never truncates silently, which puts the whole
 * burden on the reader.
 *
 * This app was reading `data` and dropping the cursor at every callsite —
 * `nextCursor` appeared in the wire types and nowhere else. So a conversation past
 * 200 items hydrated its oldest 200 and lost the rest, a 100+ session list hid the
 * remainder (and told `reconcileSessions` those sessions no longer existed, which
 * closes them), and a session past 100 runs could fail to reattach its live run.
 *
 * Stops when the runtime stops handing back a cursor — or when it hands back one
 * it already gave, since a cursor that doesn't advance is a broken server and
 * looping on it would hang the caller instead of failing.
 */
export async function eachPage<P extends Page<unknown>>(
  fetchPage: (cursor?: string) => Promise<P>,
  onPage: (page: P) => void,
): Promise<void> {
  const seen = new Set<string>();
  let cursor: string | undefined;
  for (;;) {
    const page = await fetchPage(cursor);
    onPage(page);
    const next = page.nextCursor;
    if (!next || seen.has(next)) return;
    seen.add(next);
    cursor = next;
  }
}

/** {@link eachPage}, flattened — for the callers that only want the rows. */
export async function collectPages<T>(
  fetchPage: (cursor?: string) => Promise<Page<T>>,
): Promise<T[]> {
  const rows: T[] = [];
  await eachPage(fetchPage, (page) => rows.push(...page.data));
  return rows;
}
