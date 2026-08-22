// The running kernel, as leaf code sees it. The SDK sits below the glue ring
// that owns startup, so the Host is published down here rather than imported up.
// Reads answer empty before boot; the Host is fail-closed, so there is no
// half-booted state for them to paper over.
//
// Contributions come straight off `host.contributions(token)` — Core's read for
// code outside the graph. What this adds is the app's own policy: `single`/`multi`
// resolution, the sort, and a by-reference caching contract the selectors'
// secondary indexes hang off.

import { useSyncExternalStore } from "react";
import type { ContributionView, Host } from "dougong";
import type { Contribution } from "./contracts";
import type { ExtensionPoint } from "./types/extensions";

const NOTHING: ReadonlyArray<Contribution<never>> = Object.freeze([]);
const EMPTY_NAMES: ReadonlyArray<string> = Object.freeze([]);

let host: Host | undefined;
let names: ReadonlyArray<string> = EMPTY_NAMES;
const listeners = new Set<() => void>();
const pluginNamesByHost = new WeakMap<Host, Set<string>>();

// Per point, resolved on first read and held until the kernel is retracted: the
// view's subscription is what invalidates the cached array.
let views = new Map<string, ContributionView<Contribution<unknown>>>();
let releases: Array<() => void> = [];
let entries = new Map<string, ReadonlyArray<Contribution<unknown>>>();

function announce(): void {
  names = Object.freeze([...(host ? pluginNamesFor(host) : [])].sort());
  for (const listener of [...listeners]) listener();
}

function pluginNamesFor(owner: Host): Set<string> {
  const existing = pluginNamesByHost.get(owner);
  if (existing) return existing;
  const created = new Set<string>();
  pluginNamesByHost.set(owner, created);
  return created;
}

function retractViews(): void {
  for (const release of releases) release();
  releases = [];
  views = new Map();
  entries = new Map();
}

export function publishKernel(next: Host): void {
  retractViews();
  host = next;
  announce();
}

/** Retract exactly the generation its owner is retiring. A late cleanup from an
 * older renderer must never unpublish the successor that replaced it. */
export function retractKernel(owner: Host): boolean {
  if (host !== owner) return false;
  retractViews();
  host = undefined;
  announce();
  return true;
}

/** Exact owner of the currently published contribution generation. Internal
 * readers use this identity to distinguish a Host replacement from a change
 * inside the same Host without widening the public plugin SDK. */
export function publishedKernel(): Host | undefined {
  return host;
}

export function trackInstalledPlugin(owner: Host, name: string): void {
  pluginNamesFor(owner).add(name);
  if (host === owner) announce();
}

function viewOf<T>(point: ExtensionPoint<T>): ContributionView<Contribution<T>> | undefined {
  const owner = host;
  if (!owner) return undefined;
  const cached = views.get(point.id);
  if (cached) return cached as ContributionView<Contribution<T>>;
  const view = owner.contributions(point.token);
  views.set(point.id, view as ContributionView<Contribution<unknown>>);
  const subscription = view.subscribe(() => {
    if (host !== owner) return;
    entries.delete(point.id);
    announce();
  });
  releases.push(() => subscription.dispose());
  return view;
}

// Sort hint precedence: the item's own `order` field wins, then the
// contribute-time hint, then a stable default. The sort is stable, so equal
// orders keep insertion order — which for a `single` point is what makes "last
// contributor of a key wins" mean the later plugin in the manifest.
function sortKey(entry: Contribution<unknown>): number {
  const own = (entry.item as { order?: number } | null)?.order;
  return own ?? entry.order ?? 100;
}

function resolve<T>(
  point: ExtensionPoint<T>,
  view: ContributionView<Contribution<T>>,
): ReadonlyArray<Contribution<T>> {
  const all = [...view.get().values()];
  const kept =
    point.keying === "multi"
      ? all
      : // Insertion order is contribution order, so the last writer of a key is
        // the winner. Rebuilding a Map by key keeps exactly one per key and
        // preserves each key's FIRST insertion position, which keeps the list
        // order stable while a shadowing plugin loads and unloads.
        [
          ...all
            .reduce((byKey, e) => byKey.set(e.key, e), new Map<string, Contribution<T>>())
            .values(),
        ];
  return kept.sort((a, b) => sortKey(a) - sortKey(b));
}

/**
 * Every contribution to `point`, sorted, with `single` points resolved to one
 * entry per domain key.
 *
 * Stable by reference until that point's contributions change — the contract the
 * selectors' WeakMap-keyed secondary indexes depend on.
 */
export function contributionsTo<T>(point: ExtensionPoint<T>): ReadonlyArray<Contribution<T>> {
  const cached = entries.get(point.id);
  if (cached) return cached as ReadonlyArray<Contribution<T>>;
  const view = viewOf(point);
  const resolved = view ? resolve(point, view) : NOTHING;
  entries.set(point.id, resolved as ReadonlyArray<Contribution<unknown>>);
  return resolved as ReadonlyArray<Contribution<T>>;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

/**
 * Subscribe to "some contribution changed", across kernel restarts.
 *
 * Registers against the kernel rather than one point's view, so a plugin
 * subscribing during its own setup — before any view exists — still hears about
 * later changes instead of holding a silently dead subscription.
 */
export function subscribeContributions(listener: () => void): () => void {
  return subscribe(listener);
}

function installedSnapshot(): ReadonlyArray<string> {
  return names;
}

export function useInstalledPlugins(): ReadonlyArray<string> {
  return useSyncExternalStore(subscribe, installedSnapshot, () => EMPTY_NAMES);
}

/** The resolved array is stable by reference between changes, so this re-renders
 *  exactly when the point's list is genuinely new. */
export function useContributions<T>(point: ExtensionPoint<T>): ReadonlyArray<Contribution<T>> {
  return useSyncExternalStore(
    subscribe,
    () => contributionsTo(point),
    () => NOTHING as ReadonlyArray<Contribution<T>>,
  );
}
