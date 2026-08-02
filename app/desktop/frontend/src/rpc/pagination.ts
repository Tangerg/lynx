/** The structural contract every cursor page carries. */
export interface CursorPage<T = unknown> {
  data: T[];
  nextCursor?: string;
}

export type PageItem<P extends CursorPage> = P["data"][number];

/**
 * Raised when a server returns a continuation cursor already visited by this
 * traversal. Repeating a cursor is a protocol defect: treating it as end-of-list
 * would silently turn an incomplete result into an apparently complete one.
 */
export class PaginationError extends Error {
  readonly cursor: string;

  constructor(cursor: string) {
    super(`pagination cursor did not advance: ${JSON.stringify(cursor)}`);
    this.name = "PaginationError";
    this.cursor = cursor;
  }
}

/**
 * A paged call is still a real Promise: `await call` returns its first wire page.
 * It also owns the continuation behavior generated for that method:
 *
 * - `for await (const row of call)` visits every row;
 * - `call.pages()` visits whole pages, preserving page-level side data;
 * - `call.autoPagingToArray()` collects all rows;
 * - `call.autoPagingEach(visitor)` walks rows without materializing them.
 *
 * Iteration starts from the exact cursor supplied to the original request and
 * preserves every other request field on continuation calls.
 */
export interface AutoPagingPromise<P extends CursorPage>
  extends Promise<P>, AsyncIterable<PageItem<P>> {
  pages(): AsyncIterable<P>;
  autoPagingToArray(): Promise<PageItem<P>[]>;
  autoPagingEach(
    visitor: (item: PageItem<P>) => void | boolean | Promise<void | boolean>,
  ): Promise<void>;
}

/** Build the SDK behavior for one Registry-classified cursor method. */
export function createAutoPagingPromise<P extends CursorPage>(
  fetchPage: (cursor?: string) => Promise<P>,
  initialCursor?: string,
): AutoPagingPromise<P> {
  const firstPage = fetchPage(initialCursor);

  const pages = (): AsyncIterable<P> => ({
    async *[Symbol.asyncIterator]() {
      const seen = new Set<string>();
      if (initialCursor) seen.add(initialCursor);

      let page = await firstPage;
      for (;;) {
        yield page;
        const cursor = page.nextCursor;
        if (!cursor) return;
        if (seen.has(cursor)) throw new PaginationError(cursor);
        seen.add(cursor);
        page = await fetchPage(cursor);
      }
    },
  });

  const items = async function* (): AsyncIterableIterator<PageItem<P>> {
    for await (const page of pages()) {
      yield* page.data as PageItem<P>[];
    }
  };

  const autoPagingToArray = async (): Promise<PageItem<P>[]> => {
    const result: PageItem<P>[] = [];
    for await (const item of items()) result.push(item);
    return result;
  };

  const autoPagingEach = async (
    visitor: (item: PageItem<P>) => void | boolean | Promise<void | boolean>,
  ): Promise<void> => {
    for await (const item of items()) {
      if ((await visitor(item)) === false) return;
    }
  };

  return Object.assign(firstPage, {
    pages,
    autoPagingToArray,
    autoPagingEach,
    [Symbol.asyncIterator]: items,
  });
}
