import type { LocaleSpec } from "@/plugins/sdk/types";
import { addLocaleBundle, setLocale } from "@/lib/i18n";

// Choosing a language means fetching it first. The dictionary arrives with the
// choice rather than with the app: seven bundles the reader will never see have
// no business in the entry payload, and `LocaleSpec.load` exists so a
// third-party locale plugin works exactly the same way.
export async function selectLocale(spec: LocaleSpec): Promise<void> {
  if (spec.load) addLocaleBundle(spec.id, await spec.load());
  setLocale(spec.id);
}
