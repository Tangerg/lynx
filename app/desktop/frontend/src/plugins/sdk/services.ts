// The app shell's Service contracts — what a plugin declares in `requires` to
// reach a capability the shell owns.
//
// These are what is LEFT of a fourteen-namespace Host facade after the
// contribution surfaces were taken out of it. Nine of those namespaces were not
// capabilities at all — `commands.register`, `layout.register`, `events.onStream`,
// `message.registerContentBlock`, `log.subscribe` and the rest were a second name
// for `contribute`, which is why they could sit on an interface every plugin got
// whether it used them or not. They are now what they always were: a point and a
// call to `ctx.contribute`.
//
// Two more live on the plugin context rather than here, next to Core's own `log`
// and `contribute`, because they are ambient and identity-scoped — the kernel
// gives them to every plugin and binds them to the plugin's own name, so there is
// no provider to declare and nothing a `requires` line would tell the reader:
//
//   ctx.notify(…)    attributed to this plugin in the feed
//   ctx.storage      namespaced to this plugin in localStorage
//
// Everything below is a real dependency on a real provider, and a plugin that
// wants one says so.

import { service } from "dougong";
import type { ConfigValue } from "./config";
import type { KeyValueStore } from "./storage";
import type { Disposable } from "./types/common";
import type { NotificationLevel, TaskHandle, TaskStartOptions } from "./types/infra";

export interface ConfigService {
  /** Read an app-wide config value (with optional fallback). */
  get(key: string, defaultValue?: ConfigValue): ConfigValue | undefined;
  /** Set an app-wide config value. Fires subscribers. */
  set(key: string, value: ConfigValue): void;
  /** Does the key have a value, regardless of falsiness? */
  has(key: string): boolean;
  /** Subscribe to one key. Receives the new value, or undefined when cleared. */
  onChange(key: string, fn: (value: ConfigValue | undefined) => void): Disposable;
}

export interface I18nService {
  /**
   * Merge a translation dictionary into the kernel's i18n store for `locale`.
   * Plugin keys live alongside the kernel's and resolve through `t()` normally.
   * Last writer wins on collision.
   */
  addBundle(locale: string, dict: Record<string, string>): void;
}

export interface WindowService {
  /** Set the document title's base text. Latest setter wins. */
  setTitle(text: string): void;
  /** Prefix the title with `(n)` when `n > 0`; 0 or undefined clears it. */
  setBadge(n?: number): void;
  /** Show or clear the "work in progress" dot ahead of the title. */
  setWorking(on: boolean): void;
}

export interface WorkspaceService {
  /** Open (or focus) a registered view by id. */
  openView(id: string): void;
  /** Close a registered view by id. */
  closeView(id: string): void;
}

export interface CommandsService {
  /**
   * Run a command by id — the lightweight cross-plugin call. Activates a lazy
   * command first; warns and no-ops on an unknown id.
   */
  execute(id: string, ...args: unknown[]): Promise<void>;
}

export interface PluginsService {
  /** Names of the plugins currently installed. */
  list(): ReadonlyArray<string>;
  /** Remove one, by name. No-op if it isn't installed. */
  remove(name: string): Promise<void>;
}

export const CONFIG = service<ConfigService>("lyra.shell.config");
export const I18N = service<I18nService>("lyra.shell.i18n");
export const WINDOW = service<WindowService>("lyra.shell.window");
export const WORKSPACE = service<WorkspaceService>("lyra.shell.workspace");
export const COMMANDS = service<CommandsService>("lyra.shell.commands");
export const PLUGINS = service<PluginsService>("lyra.shell.plugins");

/** The ambient half, bound per plugin by `definePlugin` — all three carry the
 *  plugin's identity, so there is no provider to declare. */
export interface AmbientShell {
  notify(message: string, level?: NotificationLevel): void;
  readonly storage: KeyValueStore;
  /** Register a long-running task; settled ones linger so the outcome is seen. */
  startTask(opts: TaskStartOptions): TaskHandle;
}
