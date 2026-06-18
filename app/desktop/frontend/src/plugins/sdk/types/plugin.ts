// PluginSpec — the top-level descriptor every plugin returns from
// `definePlugin`. Also: declarative ahead-of-activation contributions
// and the capability whitelist.

import type { ContributedCommand } from "./commands";
import type { Disposable, HostCapability } from "./common";
import type { PluginContext } from "./host";
import type { SettingsPaneSpec, WorkspaceViewSpec } from "./workspace";

/**
 * When this plugin should activate (i.e. when `setup` runs).
 *
 *   - `"onStartup"`        — load eagerly during the kernel boot sequence.
 *                            This is the default when `activationEvents`
 *                            is missing or empty.
 *   - `"onCommand:<id>"`   — activate the first time the user runs that
 *                            command (declared in `contributes.commands`).
 *
 * Future events: `"onView:<id>"`, `"onLanguage:<id>"`, etc. — add when
 * there's a real need.
 */
export type ActivationEvent = "onStartup" | `onCommand:${string}`;

/**
 * A workspace view declared ahead of activation. Same shape as
 * `WorkspaceViewSpec` minus the body component — the kernel renders a
 * lightweight "activating…" placeholder until the plugin's setup runs.
 */
export type ContributedView = Omit<WorkspaceViewSpec, "component">;

/**
 * A settings pane declared ahead of activation. Mirror of
 * `SettingsPaneSpec` minus the body component.
 */
export type ContributedSettingsPane = Omit<SettingsPaneSpec, "component">;

/**
 * Declarative ahead-of-activation contributions. Anything listed here is
 * visible in the palette / settings rail / workspace tab strip before
 * the plugin has actually been activated; first interaction triggers the
 * activation and swaps the placeholder for the real component.
 */
export interface PluginContributes {
  commands?: ContributedCommand[];
  views?: ContributedView[];
  settingsPanes?: ContributedSettingsPane[];
}

export interface PluginSpec {
  /** Unique identifier. Built-ins use the `lyra.builtin.*` namespace. */
  name: string;
  /** Semver string. Surfaced in settings + error reports. */
  version: string;
  /** Optional host API range this plugin targets. Not enforced yet. */
  apiVersion?: string;
  /**
   * Names of plugins that must load before this one. The kernel does a
   * topological sort over the requested list, then loads in dependency
   * order. Missing requires + cycles surface as setup errors.
   */
  requires?: string[];
  /**
   * When the plugin should activate. Defaults to eager (`["onStartup"]`)
   * when omitted, preserving the historical behaviour.
   */
  activationEvents?: ActivationEvent[];
  /**
   * Declarative metadata visible before activation. Today: commands,
   * workspace views, settings panes. Future surfaces follow the same
   * pattern when there's a real lazy-load use case.
   */
  contributes?: PluginContributes;
  /**
   * Capability declaration. When present, the bound host only exposes the
   * listed namespaces (others throw) and `host.extensions.contribute` only
   * accepts points whose `capability` is listed. Omit for full access
   * (built-ins). `extensions` itself is always reachable — gating is per-point.
   */
  capabilities?: HostCapability[];
  /**
   * Called once at load time. All `host.*.register*` calls go here.
   *
   * May return a cleanup function (sync or via Promise). If returned,
   * the kernel runs it when the plugin is unloaded — handy for `subscribe`
   * style side effects whose disposable isn't a `host.*.register*` result
   * (Zustand store subscriptions, window event listeners, etc.).
   */
  setup: (ctx: PluginContext) => void | (() => void) | Promise<void | (() => void)>;
}

export interface LoadedPlugin {
  spec: PluginSpec;
  disposables: Disposable[];
}

/**
 * A "plugin of plugins" — one manifest entry that owns N child plugins.
 * `definePluginPack` turns it into a regular `PluginSpec` whose setup loads the
 * children in order, runs the pack's own `setup` (so it can consume points the
 * children filled), and cascades unload in reverse. Children each get their own
 * bound host / capabilities / disposables / error isolation, and inherit the
 * pack's trust origin. A sideload pack must declare `"plugins"` (dangerous) to
 * load its children.
 */
export interface PluginPackSpec {
  name: string;
  version: string;
  apiVersion?: string;
  requires?: string[];
  capabilities?: HostCapability[];
  /** Child specs, loaded in array order (no topo-sort — order is the contract). */
  children: PluginSpec[];
  /** Optional — runs once after every child has loaded. */
  setup?: (ctx: PluginContext) => void | (() => void) | Promise<void | (() => void)>;
}
