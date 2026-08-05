/**
 * The `migrate` every persisted store here uses: keep nothing.
 *
 * `version` in this codebase means "storage written before this is dropped".
 * There are no migrations (dev phase — schema, exported shape and wire are all
 * free to change, so a stale payload is discarded, never converted), and each
 * store's `merge` already reads `undefined` as "boot from defaults".
 *
 * zustand spells "no migration" as the ABSENCE of `migrate`, which is not the
 * same statement and costs twice:
 *   - it logs `couldn't be migrated` at error level, which reads as a failure
 *     when the discard is the design; and
 *   - it treats the load as un-migrated, so storage is NOT rewritten — the
 *     stale payload and its error survive every boot until some unrelated
 *     write happens to clear it.
 *
 * Declaring the discard fixes both: the store boots on defaults and zustand
 * stamps storage at the current version on the spot. The cast is what it costs
 * to say "produces no state" in a signature that insists on one.
 */
export const discardOlderVersions = () => undefined as never;
