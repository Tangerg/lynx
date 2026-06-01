// Kernel extension points — the typed handles for every contribution
// surface the kernel itself owns. Each `host.X.register*` facade and its
// matching selector route through one of these onto the shared `extensions`
// substrate, so built-in contributions and third-party ones use the exact
// same mechanism (the JetBrains "kernel is just another extension consumer"
// property).
//
// Points are migrated domain-by-domain (L3); this file grows one block per
// migrated domain. `single` = one entry per `keyOf` (override + warn);
// `multi` = every contribution coexists.

import type {
  AgentSourceSpec,
  ComposerPlaceholderSpec,
  DataProviderSpec,
  LocaleSpec,
  PluginErrorFallbackSpec,
  RouteSpec,
  ThemeAccentSpec,
  ThemeSpec,
} from "./types";
import { defineExtensionPoint } from "./defineExtensionPoint";

// ---- theme domain --------------------------------------------------------
export const THEME = defineExtensionPoint<ThemeSpec>({ id: "lyra.theme", keying: "single" });
export const ACCENT = defineExtensionPoint<ThemeAccentSpec>({
  id: "lyra.accent",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({ id: "lyra.locale", keying: "single" });

// ---- runtime / data-layer domain -----------------------------------------
export const ROUTE = defineExtensionPoint<RouteSpec>({ id: "lyra.route", keying: "single" });
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "lyra.agent.source",
  keying: "single",
});
export const DATA_PROVIDER = defineExtensionPoint<DataProviderSpec>({
  id: "lyra.data.provider",
  keying: "single",
  keyOf: (s) => s.key,
});
export const ERROR_FALLBACK = defineExtensionPoint<PluginErrorFallbackSpec>({
  id: "lyra.plugin.errorFallback",
  keying: "single",
});

// ---- composer domain ------------------------------------------------------
export const COMPOSER_PLACEHOLDER = defineExtensionPoint<ComposerPlaceholderSpec>({
  id: "lyra.composer.placeholder",
  keying: "single",
});
