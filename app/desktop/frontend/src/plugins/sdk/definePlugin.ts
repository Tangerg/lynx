// `definePlugin` — what every plugin default-exports.
//
// A thin layer over Core's `definePlugin`, and it earns its keep by owning
// exactly one thing: `contribute`. Core's is `contribute(token, key, value)`,
// where `key` is a raw string it then qualifies with the owner. Ours takes a
// point HANDLE and applies the policy that handle carries — the capability gate,
// the key derivation (`opts.key` → `point.keyOf` → `item.id`), the key
// normalizer — and stores the result in the envelope the read side needs. Doing
// that at the call site 146 times is how the policy stops being a policy.
//
// Everything else on the context is Core's, untouched: `cleanup`, `lifetime`,
// `spawn`, `on`, `emit`, `signal`, `meta`, `log` (already bound to the plugin —
// see `hostLog`), and each declared requirement under its alias.
//
// No `version` field: a version is distribution metadata and lives on the
// platform Manifest, which is the only place anything reads one. Built-ins had
// carried `version: "1.0.0"` each and nothing ever looked.

import {
  definePlugin as defineContractPlugin,
  type AnyPlugin,
  type Awaitable,
  type PluginContext as ContractContext,
  type Provisions,
  type Requirements,
} from "dougong";
import type { Contribution } from "./contracts";
import { notifyFrom } from "./notifications";
import type { AmbientShell } from "./services";
import { startTask } from "@/state/tasksStore";
import { createStorage } from "./storage";
import type { HostCapability, Disposable } from "./types/common";
import type { ExtensionContributionOptions, ExtensionPoint } from "./types/extensions";
import type { NotificationLevel, TaskStartOptions } from "./types/infra";

/**
 * The plugin context: Core's, with `contribute` replaced by the point-aware one
 * and the two identity-scoped shell capabilities added beside Core's own `log`.
 */
export type PluginContext<Requires extends Requirements = Requirements> = Omit<
  ContractContext<Requires>,
  "contribute"
> &
  AmbientShell & {
    contribute<T>(
      point: ExtensionPoint<T>,
      item: T,
      opts?: ExtensionContributionOptions,
    ): Disposable;
  };

export interface PluginSpec<
  Requires extends Requirements = Requirements,
  Provides extends Provisions = Provisions,
> {
  /** Unique identifier. Built-ins use the `lyra.builtin.*` namespace. */
  readonly name: string;
  /**
   * Capability whitelist. When present, `contribute` only accepts points whose
   * `capability` is listed. Omit for full access (built-ins).
   */
  readonly capabilities?: ReadonlyArray<HostCapability>;
  readonly requires?: Requires;
  readonly provides?: Provides;
  readonly setup: (
    ctx: PluginContext<Requires>,
  ) => Awaitable<keyof Provides extends never ? void : { [K in keyof Provides]: unknown }>;
}

// Monotonic id minter for `multi` contributions with no explicit `opts.id`
// (event handlers, log hooks, lifecycle observers). Uniqueness only has to hold
// within one point's keyspace under one owner; a global counter is simpler than
// per-point ones and the ids never reach plugin code.
let nextMintedId = 0;

function itemId(item: unknown): string | undefined {
  if (typeof item !== "object" || item === null || !("id" in item)) return undefined;
  return typeof item.id === "string" ? item.id : undefined;
}

function domainKey<T>(
  point: ExtensionPoint<T>,
  item: T,
  opts: ExtensionContributionOptions | undefined,
): string {
  if (point.keying === "multi") return opts?.id ?? `${point.id}#${++nextMintedId}`;
  const key = opts?.key ?? point.keyOf?.(item) ?? itemId(item);
  if (!key) {
    throw new Error(
      `Single extension point "${point.id}" requires opts.key, keyOf, or a non-empty item.id`,
    );
  }
  return point.normalizeKey ? point.normalizeKey(key) : key;
}

function createContribute(
  ctx: ContractContext<Requirements>,
  name: string,
  capabilities: ReadonlyArray<HostCapability> | undefined,
) {
  return <T>(
    point: ExtensionPoint<T>,
    item: T,
    opts?: ExtensionContributionOptions,
  ): Disposable => {
    if (capabilities && point.capability && !capabilities.includes(point.capability)) {
      throw new Error(
        `[plugin] ${name}: contributing to "${point.id}" needs capability ` +
          `"${point.capability}" — add it to spec.capabilities`,
      );
    }
    const key = domainKey(point, item, opts);
    const envelope: Contribution<T> = { key, order: opts?.order, plugin: name, item };
    return ctx.contribute(point.token, key, envelope);
  };
}

// `signal` is a getter on Core's frozen context and the requirement aliases are
// dynamic, so this spreads for the aliases and re-declares `signal` as a getter
// rather than freezing whatever it happened to read at wrap time.
function bindContext<Requires extends Requirements>(
  ctx: ContractContext<Requires>,
  name: string,
  capabilities: ReadonlyArray<HostCapability> | undefined,
): PluginContext<Requires> {
  const wrapped = {
    ...ctx,
    contribute: createContribute(ctx as ContractContext<Requirements>, name, capabilities),
    notify: (message: string, level: NotificationLevel = "info") =>
      notifyFrom(name, message, level),
    storage: createStorage(name),
    startTask: (opts: TaskStartOptions) => startTask(name, opts),
  };
  Object.defineProperty(wrapped, "signal", { get: () => ctx.signal, enumerable: true });
  return Object.freeze(wrapped) as unknown as PluginContext<Requires>;
}

export function definePlugin<Requires extends Requirements = {}, Provides extends Provisions = {}>(
  spec: PluginSpec<Requires, Provides>,
): AnyPlugin {
  return defineContractPlugin<void, Requires, Provides>({
    name: spec.name,
    ...(spec.requires ? { requires: spec.requires } : {}),
    ...(spec.provides ? { provides: spec.provides } : {}),
    setup: (ctx) => spec.setup(bindContext(ctx, spec.name, spec.capabilities)) as never,
  });
}

/**
 * The minimum a registration helper needs. Helpers took the whole Host before,
 * which is how they quietly grew a second and third thing they touched.
 */
export type Contributor = Pick<PluginContext, "contribute">;
