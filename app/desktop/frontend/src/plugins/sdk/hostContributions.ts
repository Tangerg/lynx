import type {
  Disposable,
  ExtensionContributionOptions,
  ExtensionPoint,
  HostCapability,
} from "./types";
import { pluginOrigin } from "./pluginOrigin";
import { usePluginStore } from "./registry";
import { composeExtensionKey } from "./selectors/extensions";

type TrackDisposable = (disposable: Disposable) => Disposable;

// Monotonic id minter for `multi` extension-point contributions that don't
// pass an explicit `opts.id` (custom/core event handlers, rpc + log hooks,
// lifecycle observers). Uniqueness only needs to hold within one point's
// keyspace; a global counter is simpler than per-point ones and the ids
// aren't exposed to plugin code.
let nextCompositeKeyId = 0;
const mintId = (prefix: string) => `${prefix}#${++nextCompositeKeyId}`;

function itemId(item: unknown): string | undefined {
  if (typeof item !== "object" || item === null || !("id" in item)) return undefined;
  return typeof item.id === "string" ? item.id : undefined;
}

function singleContributionKey<T>(
  point: ExtensionPoint<T>,
  item: T,
  explicitKey: string | undefined,
): string {
  const key = explicitKey ?? point.keyOf?.(item) ?? itemId(item);
  if (key) return key;
  throw new Error(
    `Single extension point "${point.id}" requires opts.key, keyOf, or a non-empty item.id`,
  );
}

export function createContribute(
  pluginName: string,
  capabilities: HostCapability[] | undefined,
  track: TrackDisposable,
) {
  const store = () => usePluginStore.getState();

  // Shared write path for the open-extension-point substrate. Both the public
  // `host.extensions.contribute` and the few retained thin facades route
  // through here, so built-in and third-party contributions hit the exact same
  // code.
  return <T>(
    point: ExtensionPoint<T>,
    item: T,
    opts?: ExtensionContributionOptions,
  ): Disposable => {
    if (capabilities && point.capability && !capabilities.includes(point.capability)) {
      throw new Error(
        `[plugin] ${pluginName}: contributing to "${point.id}" needs capability ` +
          `"${point.capability}" — add it to spec.capabilities`,
      );
    }
    let outerKey: string;
    let conflictKey: string;
    if (point.keying === "single") {
      const base = singleContributionKey(point, item, opts?.key);
      conflictKey = point.normalizeKey ? point.normalizeKey(base) : base;
      outerKey = composeExtensionKey(point.id, conflictKey);
    } else {
      conflictKey = opts?.id ?? mintId(point.id);
      outerKey = composeExtensionKey(point.id, `${pluginName}|${conflictKey}`);
    }
    store().addContribution(
      pluginName,
      point.id,
      outerKey,
      { point: point.id, key: conflictKey, order: opts?.order, item },
      conflictKey,
    );
    return track({ dispose: () => store().removeContribution(pluginName, outerKey) });
  };
}

export function assertNamespaced(pluginName: string, what: string, value: string): void {
  if (pluginOrigin(pluginName) !== "sideload") return;
  const prefix = `plugin:${pluginName}/`;
  if (!value.startsWith(prefix)) {
    console.warn(
      `[plugin:${pluginName}] ${what} "${value}" is not namespaced; ` +
        `third-party ${what}s must be "${prefix}<symbol>" (API.md §2.5)`,
    );
  }
}
